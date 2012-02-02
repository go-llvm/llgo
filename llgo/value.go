/*
Copyright (c) 2011, 2012 Andrew Wilkins <axwalk@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package llgo

import (
    "go/token"
    "github.com/axw/gollvm/llvm"
    "math"
    "big"
    "fmt"
)

var (
    maxBigInt32 = big.NewInt(math.MaxInt32)
    minBigInt32 = big.NewInt(math.MinInt32)
)

// Value is an interface for representing values returned by Go expressions.
type Value interface {
    // BinaryOp applies the specified binary operator to this value and the
    // specified right-hand operand, and returns a new Value.
    BinaryOp(op token.Token, rhs Value) Value

    // UnaryOp applies the specified unary operator and returns a new Value.
    UnaryOp(op token.Token) Value

    // Convert returns a new Value which has been converted to the specified
    // type.
    Convert(typ Type) Value

    // LLVMValue returns an llvm.Value for this value.
    LLVMValue() llvm.Value

    // Type returns the Type of the value.
    Type() Type
}

type LLVMValue struct {
    builder  llvm.Builder
    value    llvm.Value
    typ      Type
    indirect bool
}

type ConstValue struct {
    Const // from go_const.go (ripped from go/types)
    typ *Basic
}

// Create a new dynamic value from a (LLVM Builder, LLVM Value, Type) triplet.
func NewLLVMValue(b llvm.Builder, v llvm.Value, t Type) *LLVMValue {
    return &LLVMValue{b, v, t, false}
}

// Create a new constant value from a literal with accompanying type, as
// provided by ast.BasicLit.
func NewConstValue(tok token.Token, lit string) ConstValue {
    var typ *Basic
    switch tok {
    case token.INT:    typ = &Basic{Kind: UntypedInt}
    case token.FLOAT:  typ = &Basic{Kind: UntypedFloat}
    case token.IMAG:   typ = &Basic{Kind: UntypedComplex}
    case token.CHAR:   typ = &Basic{Kind: Int} // XXX rune
    case token.STRING: typ = &Basic{Kind: String}
    }
    return ConstValue{MakeConst(tok, lit), typ}
}

///////////////////////////////////////////////////////////////////////////////
// LLVMValue methods

func (lhs *LLVMValue) BinaryOp(op token.Token, rhs_ Value) Value {
    // Deref lhs, if it's indirect.
    if lhs.indirect {
        lhs = lhs.Deref()
    }

    switch rhs := rhs_.(type) {
    case *LLVMValue:
        // Deref rhs, if it's indirect.
        if rhs.indirect {
            rhs = rhs.Deref()
        }

        b := lhs.builder
        var result llvm.Value
        switch op {
        case token.MUL:
            result = b.CreateMul(lhs.value, rhs.value, "")
        case token.QUO:
            result = b.CreateUDiv(lhs.value, rhs.value, "")
        case token.ADD:
            result = b.CreateAdd(lhs.value, rhs.value, "")
        case token.SUB:
            result = b.CreateSub(lhs.value, rhs.value, "")
        case token.EQL:
            result = b.CreateICmp(llvm.IntEQ, lhs.value, rhs.value, "")
        case token.LSS:
            result = b.CreateICmp(llvm.IntULT, lhs.value, rhs.value, "")
        default:
            panic("Unimplemented")
        }
        return NewLLVMValue(b, result, lhs.typ)
    case ConstValue:
        // TODO
    }
    panic("unimplemented")
}

func (v *LLVMValue) UnaryOp(op token.Token) Value {
    b := v.builder
    var result llvm.Value
    switch op {
    case token.SUB:
        result = b.CreateNeg(v.value, "")
    case token.ADD:
        result = v.value // No-op
    default:
        panic("Unhandled operator: ")// + expr.Op)
    }
    return NewLLVMValue(b, result, v.typ)
}

func (v *LLVMValue) Convert(typ Type) Value {
    if v.typ == typ {
        return v
    }
/*
    value_type := value.Type()
    switch value_type.TypeKind() {
    case llvm.IntegerTypeKind:
        switch totype.TypeKind() {
        case llvm.IntegerTypeKind:
            //delta := value_type.IntTypeWidth() - totype.IntTypeWidth()
            //var 
            switch {
            case delta == 0: return value
            // TODO handle signed/unsigned (SExt/ZExt)
            case delta < 0: return c.builder.CreateZExt(value, totype, "")
            case delta > 0: return c.builder.CreateTrunc(value, totype, "")
            }
            return LLVMValue{lhs.builder, value}
        }
    }
*/
    panic("unimplemented")
}

func (v *LLVMValue) LLVMValue() llvm.Value {
    return v.value
}

func (v *LLVMValue) Type() Type {
    return v.typ
}

// Dereference an LLVMValue, producing a new LLVMValue.
func (v *LLVMValue) Deref() *LLVMValue {
    llvm_value := v.builder.CreateLoad(v.value, "")
    return NewLLVMValue(v.builder, llvm_value, Deref(v.typ))
}

///////////////////////////////////////////////////////////////////////////////
// ConstValue methods.

func (lhs ConstValue) BinaryOp(op token.Token, rhs_ Value) Value {
    switch rhs := rhs_.(type) {
    case *LLVMValue:
    case ConstValue:
        // TODO Check if either one is untyped, and convert to the other's
        // type.
        typ := lhs.typ
        return ConstValue{lhs.Const.BinaryOp(op, rhs.Const), typ}
    }
    panic("unimplemented")
}

func (v ConstValue) UnaryOp(op token.Token) Value {
    return ConstValue{v.Const.UnaryOp(op), v.typ}
}

func (v ConstValue) Convert(typ Type) Value {
    if v.typ != typ {
        return ConstValue{v.Const.Convert(&typ), typ.(*Basic)}
    }
    return v
}

func (v ConstValue) LLVMValue() llvm.Value {
    // From the language spec:
    //   If the type is absent and the corresponding expression evaluates to
    //   an untyped constant, the type of the declared variable is bool, int,
    //   float64, or string respectively, depending on whether the value is
    //   a boolean, integer, floating-point, or string constant.

    switch v.typ.Kind {
    case UntypedInt:
        // TODO 32/64bit
        int_val := v.val.(*big.Int)
        if int_val.Cmp(maxBigInt32) > 0 || int_val.Cmp(minBigInt32) < 0 {
            panic(fmt.Sprint("const ", int_val, " overflows int"))
        }
        return llvm.ConstInt(llvm.Int32Type(), uint64(v.Int64()), false)
    case UntypedFloat: fallthrough
    case UntypedComplex:
        panic("Attempting to take LLVM value of untyped constant")
    case Int:
        // XXX rune
        // FIXME use int32/int64
        return llvm.ConstInt(llvm.Int32Type(), uint64(v.Int64()), false)
    case String:
        return llvm.ConstString((v.val).(string), true)
    }
    panic("Unhandled type")
}

func (v ConstValue) Type() Type {
    // TODO convert untyped to typed?
    switch v.typ.Kind {
    case UntypedInt: return IntType
    }
    return v.typ
}

func (v ConstValue) Int64() int64 {
    int_val := v.val.(*big.Int)
    return int_val.Int64()
}

// vim: set ft=go :

