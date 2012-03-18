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
	"fmt"
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo/types"
	"go/token"
	"math"
	"math/big"
	"sort"
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
	Convert(typ types.Type) Value

	// LLVMValue returns an llvm.Value for this value.
	LLVMValue() llvm.Value

	// Type returns the Type of the value.
	Type() types.Type
}

type LLVMValue struct {
	compiler *compiler
	value    llvm.Value
	typ      types.Type
	indirect bool
	address  *LLVMValue // Value that dereferenced to this value.
	receiver *LLVMValue
}

type ConstValue struct {
	types.Const
	compiler *compiler
	typ      *types.Basic
}

// Create a new dynamic value from a (LLVM Builder, LLVM Value, Type) triplet.
func (c *compiler) NewLLVMValue(v llvm.Value, t types.Type) *LLVMValue {
	return &LLVMValue{c, v, t, false, nil, nil}
}

// Create a new constant value from a literal with accompanying type, as
// provided by ast.BasicLit.
func (c *compiler) NewConstValue(tok token.Token, lit string) ConstValue {
	var typ *types.Basic
	switch tok {
	case token.INT:
		typ = &types.Basic{Kind: types.UntypedIntKind}
	case token.FLOAT:
		typ = &types.Basic{Kind: types.UntypedFloatKind}
	case token.IMAG:
		typ = &types.Basic{Kind: types.UntypedComplexKind}
	case token.CHAR:
		typ = &types.Basic{Kind: types.Int32Kind} // rune
	case token.STRING:
		typ = &types.Basic{Kind: types.StringKind}
	}
	return ConstValue{*types.MakeConst(tok, lit), c, typ}
}

///////////////////////////////////////////////////////////////////////////////
// LLVMValue methods

func (lhs *LLVMValue) BinaryOp(op token.Token, rhs_ Value) Value {
	// Deref lhs, if it's indirect.
	if lhs.indirect {
		lhs = lhs.Deref()
	}

	var result llvm.Value
	b := lhs.compiler.builder

	switch rhs := rhs_.(type) {
	case *LLVMValue:
		// Deref rhs, if it's indirect.
		if rhs.indirect {
			rhs = rhs.Deref()
		}

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
		case token.LEQ: // TODO signed/unsigned
			result = b.CreateICmp(llvm.IntULE, lhs.value, rhs.value, "")
		default:
			panic("Unimplemented")
		}
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	case ConstValue:
		// Cast untyped rhs to lhs type.
		switch rhs.typ.Kind {
		case types.UntypedIntKind:
			fallthrough
		case types.UntypedFloatKind:
			fallthrough
		case types.UntypedComplexKind:
			rhs = rhs.Convert(lhs.Type()).(ConstValue)
		}
		rhs_value := rhs.LLVMValue()

		switch op {
		case token.MUL:
			result = b.CreateMul(lhs.value, rhs_value, "")
		case token.QUO:
			result = b.CreateUDiv(lhs.value, rhs_value, "")
		case token.ADD:
			result = b.CreateAdd(lhs.value, rhs_value, "")
		case token.SUB:
			result = b.CreateSub(lhs.value, rhs_value, "")
		case token.EQL:
			result = b.CreateICmp(llvm.IntEQ, lhs.value, rhs_value, "")
		case token.LSS:
			result = b.CreateICmp(llvm.IntULT, lhs.value, rhs_value, "")
		case token.LEQ: // TODO signed/unsigned
			result = b.CreateICmp(llvm.IntULE, lhs.value, rhs_value, "")
		default:
			panic("Unimplemented")
		}
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	}
	panic("unimplemented")
}

func (v *LLVMValue) UnaryOp(op token.Token) Value {
	b := v.compiler.builder
	switch op {
	case token.SUB:
		if v.indirect {
			v2 := v.Deref()
			return v.compiler.NewLLVMValue(b.CreateNeg(v2.value, ""), v2.typ)
		}
		return v.compiler.NewLLVMValue(b.CreateNeg(v.value, ""), v.typ)
	case token.ADD:
		return v // No-op
	case token.AND:
		if v.indirect {
			return v.compiler.NewLLVMValue(v.value, v.typ)
		}
		return v.compiler.NewLLVMValue(v.address.LLVMValue(),
			&types.Pointer{Base: v.typ})
	default:
		panic("Unhandled operator: ") // + expr.Op)
	}
	panic("unreachable")
}

func (v *LLVMValue) Convert(typ types.Type) Value {
	if types.Identical(v.typ, typ) {
		return v
	}

	if name, isname := typ.(*types.Name); isname {
		typ = name.Underlying
	}

	var srcname *types.Name
	srctyp := v.typ
	if name, isname := srctyp.(*types.Name); isname {
		srcname = name
		srctyp = name.Underlying
	}

	// Converting to an interface type.
	if interface_, isinterface := typ.(*types.Interface); isinterface {
		isptr := false
		if p, fromptr := srctyp.(*types.Pointer); fromptr {
			isptr = true
			srctyp = p.Base
			if name, isname := srctyp.(*types.Name); isname {
				srcname = name
				srctyp = name.Underlying
			}
		}

		if _, fromstruct := srctyp.(*types.Struct);
		   srcname != nil && fromstruct {
			// TODO check whether the functions in the struct take
			// value or pointer receivers.

			iface_elements := make([]llvm.Value, 1+len(interface_.Methods))

			iface_struct_type := interface_.LLVMType()
			element_types := iface_struct_type.StructElementTypes()
			for i, _ := range iface_elements {
				iface_elements[i] = llvm.ConstNull(element_types[i])
			}
			iface_struct := llvm.ConstStruct(iface_elements, false)

			builder := v.compiler.builder
			var ptr llvm.Value
			if isptr {
				ptr = v.LLVMValue()
			} else {
				ptr = builder.CreateMalloc(srctyp.LLVMType(), "")
			}
			ptr = builder.CreateBitCast(ptr, element_types[0], "")
			iface_struct = builder.CreateInsertValue(iface_struct, ptr, 0, "")

			// Look up the method by name.
			methods := srcname.Methods
			for i, m := range interface_.Methods {
				// TODO make this loop linear by iterating through the
				// interface methods and type methods together.
				mi := sort.Search(len(methods), func(i int) bool {
					return methods[i].Name >= m.Name
				})
				if mi >= len(methods) || methods[mi].Name != m.Name {
						panic("Failed to locate method: " + m.Name)
				}
				method_obj := methods[mi]
				method := v.compiler.Resolve(method_obj).(*LLVMValue)
				llvm_value := method.LLVMValue()
				llvm_value = builder.CreateBitCast(
						llvm_value,
						element_types[i+1], "")
				iface_struct = builder.CreateInsertValue(
						iface_struct, llvm_value,
						i+1, "")
			}

			return v.compiler.NewLLVMValue(iface_struct, interface_)

			//iface_struct := v.compiler.builder.CreateMalloc(
			//    iface_struct_type, "")
			//return NewLLVMValue(v.compiler, iface_struct,
			//                    &types.Pointer{Base: interface_})
		}
		// TODO handle other interface conversions (e.g. one interface to
		// another).
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
	           case delta < 0: return c.compiler.builder.CreateZExt(value, totype, "")
	           case delta > 0: return c.compiler.builder.CreateTrunc(value, totype, "")
	           }
	           return LLVMValue{lhs.compiler.builder, value}
	       }
	   }
	*/
	panic(fmt.Sprint("unimplemented conversion: ", v.typ, " -> ", typ))
}

func (v *LLVMValue) LLVMValue() llvm.Value {
	return v.value
}

func (v *LLVMValue) Type() types.Type {
	return v.typ
}

// Dereference an LLVMValue, producing a new LLVMValue.
func (v *LLVMValue) Deref() *LLVMValue {
	llvm_value := v.compiler.builder.CreateLoad(v.value, "")
	value := v.compiler.NewLLVMValue(llvm_value, types.Deref(v.typ))
	value.address = v
	return value
}

///////////////////////////////////////////////////////////////////////////////
// ConstValue methods.

func (lhs ConstValue) BinaryOp(op token.Token, rhs_ Value) Value {
	switch rhs := rhs_.(type) {
	case *LLVMValue:
		// Deref rhs, if it's indirect.
		if rhs.indirect {
			rhs = rhs.Deref()
		}

		// Cast untyped lhs to rhs type.
		switch lhs.typ.Kind {
		case types.UntypedIntKind:
			fallthrough
		case types.UntypedFloatKind:
			fallthrough
		case types.UntypedComplexKind:
			lhs = lhs.Convert(rhs.Type()).(ConstValue)
		}
		lhs_value := lhs.LLVMValue()

		b := rhs.compiler.builder
		var result llvm.Value
		switch op {
		case token.MUL:
			result = b.CreateMul(lhs_value, rhs.value, "")
		case token.QUO:
			result = b.CreateUDiv(lhs_value, rhs.value, "")
		case token.ADD:
			result = b.CreateAdd(lhs_value, rhs.value, "")
		case token.SUB:
			result = b.CreateSub(lhs_value, rhs.value, "")
		case token.EQL:
			result = b.CreateICmp(llvm.IntEQ, lhs_value, rhs.value, "")
		case token.LSS:
			result = b.CreateICmp(llvm.IntULT, lhs_value, rhs.value, "")
		default:
			panic("Unimplemented")
		}
		return rhs.compiler.NewLLVMValue(result, lhs.typ)
	case ConstValue:
		// TODO Check if either one is untyped, and convert to the other's
		// type.
		c := lhs.compiler
		typ := lhs.typ
		return ConstValue{*lhs.Const.BinaryOp(op, &rhs.Const), c, typ}
	}
	panic("unimplemented")
}

func (v ConstValue) UnaryOp(op token.Token) Value {
	return ConstValue{*v.Const.UnaryOp(op), v.compiler, v.typ}
}

func (v ConstValue) Convert(typ types.Type) Value {
	if !types.Identical(v.typ, typ) {
		if name, isname := typ.(*types.Name); isname {
			typ = name.Underlying
		}
		compiler := v.compiler
		return ConstValue{*v.Const.Convert(&typ), compiler, typ.(*types.Basic)}
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
	case types.UntypedIntKind:
		// TODO 32/64bit
		int_val := v.Val.(*big.Int)
		if int_val.Cmp(maxBigInt32) > 0 || int_val.Cmp(minBigInt32) < 0 {
			panic(fmt.Sprint("const ", int_val, " overflows int"))
		}
		return llvm.ConstInt(llvm.Int32Type(), uint64(v.Int64()), false)
	case types.UntypedFloatKind:
		fallthrough
	case types.UntypedComplexKind:
		panic("Attempting to take LLVM value of untyped constant")
	case types.Int32Kind, types.Uint32Kind:
		// XXX rune
		return llvm.ConstInt(llvm.Int32Type(), uint64(v.Int64()), false)
	case types.Int16Kind, types.Uint16Kind:
		return llvm.ConstInt(llvm.Int16Type(), uint64(v.Int64()), false)
	case types.StringKind:
		strval := (v.Val).(string)
		ptr := v.compiler.builder.CreateGlobalStringPtr(strval, "")
		len_ := llvm.ConstInt(llvm.Int32Type(), uint64(len(strval)), false)
		return llvm.ConstStruct([]llvm.Value{ptr, len_}, false)
	case types.BoolKind:
		if v := v.Val.(bool); v {
			return llvm.ConstAllOnes(llvm.Int1Type())
		}
		return llvm.ConstNull(llvm.Int1Type())
	}
	panic("Unhandled type")
}

func (v ConstValue) Type() types.Type {
	// TODO convert untyped to typed?
	switch v.typ.Kind {
	case types.UntypedIntKind:
		return types.Int
	}
	return v.typ
}

func (v ConstValue) Int64() int64 {
	int_val := v.Val.(*big.Int)
	return int_val.Int64()
}

// vim: set ft=go :
