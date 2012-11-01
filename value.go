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
	"go/ast"
	"go/token"
	"math"
	"math/big"
)

var (
	maxBigInt32 = big.NewInt(math.MaxInt32)
	minBigInt32 = big.NewInt(math.MinInt32)
)

// Resolver is an interface for resolving AST objects to values.
type Resolver interface {
	Resolve(*ast.Object) Value
}

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

// LLVMValue represents a dynamic value produced as the result of an
// expression.
type LLVMValue struct {
	compiler *compiler
	value    llvm.Value
	typ      types.Type
	pointer  *LLVMValue // Pointer value that dereferenced to this value.
	receiver *LLVMValue // Transient; only guaranteed to exist at call point.
}

// ConstValue represents a constant value produced as the result of an
// expression.
type ConstValue struct {
	types.Const
	compiler *compiler
	typ      types.Type
}

// TypeValue represents a Type result of an expression. All methods
// other than Type() will panic when called.
type TypeValue struct {
	typ types.Type
}

// NilValue represents a nil value. All methods other than Convert will
// panic when called.
type NilValue struct {
	compiler *compiler
}

// Create a new dynamic value from a (LLVM Builder, LLVM Value, Type) triplet.
func (c *compiler) NewLLVMValue(v llvm.Value, t types.Type) *LLVMValue {
	return &LLVMValue{c, v, t, nil, nil}
}

// Create a new constant value from a literal with accompanying type, as
// provided by ast.BasicLit.
func (c *compiler) NewConstValue(tok token.Token, lit string) ConstValue {
	var typ types.Type
	switch tok {
	case token.INT:
		typ = types.Int.Underlying
	case token.FLOAT:
		typ = types.Float64.Underlying
	case token.IMAG:
		typ = types.Complex128.Underlying
	case token.CHAR:
		typ = types.Rune.Underlying
	case token.STRING:
		typ = types.String.Underlying
	}
	return ConstValue{types.MakeConst(tok, lit), c, typ}
}

///////////////////////////////////////////////////////////////////////////////
// LLVMValue methods

func signed(typ types.Type) bool {
	if typ_, ok := typ.(*types.Name); ok {
		typ = typ_.Underlying
	}
	if basic, ok := typ.(*types.Basic); ok {
		switch basic.Kind {
		case types.IntKind, types.Int8Kind, types.Int16Kind,
			types.Int32Kind, types.Int64Kind:
			return true
		}
	}
	return false
}

func (lhs *LLVMValue) BinaryOp(op token.Token, rhs_ Value) Value {
	if op == token.NEQ {
		result := lhs.BinaryOp(token.EQL, rhs_)
		return result.UnaryOp(token.NOT)
	}

	var result llvm.Value
	c := lhs.compiler
	b := lhs.compiler.builder

	// Later we can do better by treating constants specially. For now, let's
	// convert to LLVMValue's.
	var rhs *LLVMValue
	rhsisnil := false
	switch rhs_ := rhs_.(type) {
	case *LLVMValue:
		rhs = rhs_
	case NilValue:
		rhsisnil = true
		switch rhs_ := rhs_.Convert(lhs.Type()).(type) {
		case ConstValue:
			rhs = c.NewLLVMValue(rhs_.LLVMValue(), rhs_.Type())
		case *LLVMValue:
			rhs = rhs_
		}
	case ConstValue:
		value := rhs_.Convert(lhs.Type())
		rhs = c.NewLLVMValue(value.LLVMValue(), value.Type())
	}

	switch typ := types.Underlying(lhs.typ).(type) {
	case *types.Struct:
		// TODO check types are the same.
		element_types_count := lhs.LLVMValue().Type().StructElementTypesCount()
		struct_fields := typ.Fields
		if element_types_count > 0 {
			t := c.ObjGetType(struct_fields[0])
			first_lhs := c.NewLLVMValue(b.CreateExtractValue(lhs.LLVMValue(), 0, ""), t)
			first_rhs := c.NewLLVMValue(b.CreateExtractValue(rhs.LLVMValue(), 0, ""), t)
			first := first_lhs.BinaryOp(op, first_rhs)

			logical_op := token.LAND
			if op == token.NEQ {
				logical_op = token.LOR
			}

			result := first
			for i := 1; i < element_types_count; i++ {
				t := c.ObjGetType(struct_fields[i])
				next_lhs := c.NewLLVMValue(b.CreateExtractValue(lhs.LLVMValue(), i, ""), t)
				next_rhs := c.NewLLVMValue(b.CreateExtractValue(rhs.LLVMValue(), i, ""), t)
				next := next_lhs.BinaryOp(op, next_rhs)
				result = result.BinaryOp(logical_op, next)
			}
			return result
		}

	case *types.Interface:
		if rhsisnil {
			valueNull := b.CreateIsNull(b.CreateExtractValue(lhs.LLVMValue(), 0, ""), "")
			typeNull := b.CreateIsNull(b.CreateExtractValue(lhs.LLVMValue(), 1, ""), "")
			result := b.CreateAnd(typeNull, valueNull, "")
			return c.NewLLVMValue(result, types.Bool)
		}
		// TODO check for interface/interface comparison vs. interface/value comparison.
		return lhs.compareI2I(rhs)

	case *types.Slice:
		// []T == nil
		isnil := b.CreateIsNull(b.CreateExtractValue(lhs.LLVMValue(), 0, ""), "")
		return c.NewLLVMValue(isnil, types.Bool)
	}

	// Strings.
	if types.Underlying(lhs.typ) == types.String {
		if types.Underlying(rhs.typ) == types.String {
			switch op {
			case token.ADD:
				return c.concatenateStrings(lhs, rhs)
			case token.EQL, token.LSS, token.GTR, token.LEQ, token.GEQ:
				return c.compareStrings(lhs, rhs, op)
			default:
				panic(fmt.Sprint("Unimplemented operator: ", op))
			}
		}
		panic("unimplemented")
	}

	// Numbers.
	// Determine whether to use integer or floating point instructions.
	// TODO determine the NaN rules.
	isfp := types.Identical(types.Underlying(lhs.typ), types.Float32) ||
		types.Identical(types.Underlying(lhs.typ), types.Float64)

	switch op {
	case token.MUL:
		if isfp {
			result = b.CreateFMul(lhs.LLVMValue(), rhs.LLVMValue(), "")
		} else {
			result = b.CreateMul(lhs.LLVMValue(), rhs.LLVMValue(), "")
		}
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	case token.QUO:
		if isfp {
			result = b.CreateFDiv(lhs.LLVMValue(), rhs.LLVMValue(), "")
		} else {
			result = b.CreateUDiv(lhs.LLVMValue(), rhs.LLVMValue(), "")
		}
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	case token.REM:
		if isfp {
			result = b.CreateFRem(lhs.LLVMValue(), rhs.LLVMValue(), "")
		} else {
			result = b.CreateURem(lhs.LLVMValue(), rhs.LLVMValue(), "")
		}
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	case token.ADD:
		if isfp {
			result = b.CreateFAdd(lhs.LLVMValue(), rhs.LLVMValue(), "")
		} else {
			result = b.CreateAdd(lhs.LLVMValue(), rhs.LLVMValue(), "")
		}
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	case token.SUB:
		if isfp {
			result = b.CreateFSub(lhs.LLVMValue(), rhs.LLVMValue(), "")
		} else {
			result = b.CreateSub(lhs.LLVMValue(), rhs.LLVMValue(), "")
		}
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	case token.SHL:
		rhs = rhs.Convert(lhs.Type()).(*LLVMValue)
		result = b.CreateShl(lhs.LLVMValue(), rhs.LLVMValue(), "")
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	case token.SHR:
		rhs = rhs.Convert(lhs.Type()).(*LLVMValue)
		result = b.CreateAShr(lhs.LLVMValue(), rhs.LLVMValue(), "")
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	case token.NEQ:
		if isfp {
			result = b.CreateFCmp(llvm.FloatONE, lhs.LLVMValue(), rhs.LLVMValue(), "")
		} else {
			result = b.CreateICmp(llvm.IntNE, lhs.LLVMValue(), rhs.LLVMValue(), "")
		}
		return lhs.compiler.NewLLVMValue(result, types.Bool)
	case token.EQL:
		if isfp {
			result = b.CreateFCmp(llvm.FloatOEQ, lhs.LLVMValue(), rhs.LLVMValue(), "")
		} else {
			result = b.CreateICmp(llvm.IntEQ, lhs.LLVMValue(), rhs.LLVMValue(), "")
		}
		return lhs.compiler.NewLLVMValue(result, types.Bool)
	case token.LSS:
		if isfp {
			result = b.CreateFCmp(llvm.FloatOLT, lhs.LLVMValue(), rhs.LLVMValue(), "")
		} else {
			result = b.CreateICmp(llvm.IntULT, lhs.LLVMValue(), rhs.LLVMValue(), "")
		}
		return lhs.compiler.NewLLVMValue(result, types.Bool)
	case token.LEQ: // TODO signed/unsigned
		if isfp {
			result = b.CreateFCmp(llvm.FloatOLE, lhs.LLVMValue(), rhs.LLVMValue(), "")
		} else {
			result = b.CreateICmp(llvm.IntULE, lhs.LLVMValue(), rhs.LLVMValue(), "")
		}
		return lhs.compiler.NewLLVMValue(result, types.Bool)
	case token.GTR:
		if isfp {
			result = b.CreateFCmp(llvm.FloatOGT, lhs.LLVMValue(), rhs.LLVMValue(), "")
		} else {
			result = b.CreateICmp(llvm.IntUGT, lhs.LLVMValue(), rhs.LLVMValue(), "")
		}
		return lhs.compiler.NewLLVMValue(result, types.Bool)
	case token.GEQ:
		if isfp {
			result = b.CreateFCmp(llvm.FloatOGE, lhs.LLVMValue(), rhs.LLVMValue(), "")
		} else {
			result = b.CreateICmp(llvm.IntUGE, lhs.LLVMValue(), rhs.LLVMValue(), "")
		}
		return lhs.compiler.NewLLVMValue(result, types.Bool)
	case token.AND: // a & b
		result = b.CreateAnd(lhs.LLVMValue(), rhs.LLVMValue(), "")
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	case token.AND_NOT: // a &^ b
		rhsval := rhs.LLVMValue()
		rhsval = b.CreateXor(rhsval, llvm.ConstAllOnes(rhsval.Type()), "")
		result = b.CreateAnd(lhs.LLVMValue(), rhsval, "")
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	case token.OR: // a | b
		result = b.CreateOr(lhs.LLVMValue(), rhs.LLVMValue(), "")
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	case token.XOR: // a ^ b
		result = b.CreateXor(lhs.LLVMValue(), rhs.LLVMValue(), "")
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	default:
		panic(fmt.Sprint("Unimplemented operator: ", op))
	}
	panic("unreachable")
}

func (v *LLVMValue) UnaryOp(op token.Token) Value {
	b := v.compiler.builder
	switch op {
	case token.SUB:
		var value llvm.Value
		isfp := types.Identical(types.Underlying(v.typ), types.Float32) ||
			types.Identical(types.Underlying(v.typ), types.Float64)
		if isfp {
			zero := llvm.ConstNull(v.compiler.types.ToLLVM(v.Type()))
			value = b.CreateFSub(zero, v.LLVMValue(), "")
		} else {
			value = b.CreateNeg(v.LLVMValue(), "")
		}
		return v.compiler.NewLLVMValue(value, v.typ)
	case token.ADD:
		return v // No-op
	case token.AND:
		return v.pointer
	case token.NOT:
		value := b.CreateNot(v.LLVMValue(), "")
		return v.compiler.NewLLVMValue(value, v.typ)
	case token.XOR:
		lhs := v.LLVMValue()
		rhs := llvm.ConstAllOnes(lhs.Type())
		value := b.CreateXor(lhs, rhs, "")
		return v.compiler.NewLLVMValue(value, v.typ)
	case token.ARROW:
		return v.chanRecv()
	default:
		panic(fmt.Sprintf("Unhandled operator: %s", op))
	}
	panic("unreachable")
}

func (v *LLVMValue) Convert(dst_typ types.Type) Value {
	// If it's a stack allocated value, we'll want to compare the
	// value type, not the pointer type.
	src_typ := v.typ

	// Get the underlying type, if any.
	orig_dst_typ := dst_typ
	dst_typ = types.Underlying(dst_typ)
	src_typ = types.Underlying(src_typ)

	// Identical (underlying) types? Just swap in the destination type.
	if types.Identical(src_typ, dst_typ) {
		// TODO avoid load here by reusing pointer value, if exists.
		return v.compiler.NewLLVMValue(v.LLVMValue(), orig_dst_typ)
	}

	// Both pointer types with identical underlying types? Same as above.
	if src_typ, ok := src_typ.(*types.Pointer); ok {
		if dst_typ, ok := dst_typ.(*types.Pointer); ok {
			src_typ := types.Underlying(src_typ.Base)
			dst_typ := types.Underlying(dst_typ.Base)
			if types.Identical(src_typ, dst_typ) {
				return v.compiler.NewLLVMValue(v.LLVMValue(), orig_dst_typ)
			}
		}
	}

	// Convert from an interface type.
	if _, isinterface := src_typ.(*types.Interface); isinterface {
		if interface_, isinterface := dst_typ.(*types.Interface); isinterface {
			result, _ := v.convertI2I(interface_)
			return result
		} else {
			return v.convertI2V(dst_typ)
		}
	}

	// Converting to an interface type.
	if interface_, isinterface := dst_typ.(*types.Interface); isinterface {
		return v.convertV2I(interface_)
	}

	// string -> []byte
	byteslice := &types.Slice{Elt: types.Byte}
	if src_typ == types.String && types.Identical(dst_typ, byteslice) {
		c := v.compiler
		value := v.LLVMValue()
		strdata := c.builder.CreateExtractValue(value, 0, "")
		strlen := c.builder.CreateExtractValue(value, 1, "")
		struct_ := llvm.Undef(c.types.ToLLVM(byteslice))
		struct_ = c.builder.CreateInsertValue(struct_, strdata, 0, "")
		struct_ = c.builder.CreateInsertValue(struct_, strlen, 1, "")
		struct_ = c.builder.CreateInsertValue(struct_, strlen, 2, "")
		return c.NewLLVMValue(struct_, byteslice)
	}
	// []byte -> string
	if types.Identical(src_typ, byteslice) && dst_typ == types.String {
		c := v.compiler
		value := v.LLVMValue()
		data := c.builder.CreateExtractValue(value, 0, "")
		len := c.builder.CreateExtractValue(value, 1, "")
		struct_ := llvm.Undef(c.types.ToLLVM(types.String))
		struct_ = c.builder.CreateInsertValue(struct_, data, 0, "")
		struct_ = c.builder.CreateInsertValue(struct_, len, 1, "")
		return c.NewLLVMValue(struct_, types.String)
	}

	// TODO other special conversions, e.g. int->string.
	llvm_type := v.compiler.types.ToLLVM(dst_typ)

	// Unsafe pointer conversions.
	if dst_typ == types.UnsafePointer { // X -> unsafe.Pointer
		if _, isptr := src_typ.(*types.Pointer); isptr {
			value := v.compiler.builder.CreatePtrToInt(v.LLVMValue(), llvm_type, "")
			return v.compiler.NewLLVMValue(value, dst_typ)
		} else if src_typ == types.Uintptr {
			return v.compiler.NewLLVMValue(v.LLVMValue(), dst_typ)
		}
	} else if src_typ == types.UnsafePointer { // unsafe.Pointer -> X
		if _, isptr := dst_typ.(*types.Pointer); isptr {
			value := v.compiler.builder.CreateIntToPtr(v.LLVMValue(), llvm_type, "")
			return v.compiler.NewLLVMValue(value, dst_typ)
		} else if dst_typ == types.Uintptr {
			return v.compiler.NewLLVMValue(v.LLVMValue(), dst_typ)
		}
	}

	// FIXME select the appropriate cast here, depending on size, type (int/float)
	// and sign.
	lv := v.LLVMValue()
	srcType := lv.Type()
	switch srcType.TypeKind() { // source type
	case llvm.IntegerTypeKind:
		switch llvm_type.TypeKind() {
		case llvm.IntegerTypeKind:
			srcBits := srcType.IntTypeWidth()
			dstBits := llvm_type.IntTypeWidth()
			delta := srcBits - dstBits
			switch {
			case delta < 0:
				// TODO check if (un)signed, use S/ZExt accordingly.
				lv = v.compiler.builder.CreateZExt(lv, llvm_type, "")
			case delta > 0:
				lv = v.compiler.builder.CreateTrunc(lv, llvm_type, "")
			}
			return v.compiler.NewLLVMValue(lv, dst_typ)
		case llvm.FloatTypeKind, llvm.DoubleTypeKind:
			if signed(v.Type()) {
				lv = v.compiler.builder.CreateSIToFP(lv, llvm_type, "")
			} else {
				lv = v.compiler.builder.CreateUIToFP(lv, llvm_type, "")
			}
			return v.compiler.NewLLVMValue(lv, dst_typ)
		}
	case llvm.DoubleTypeKind:
		switch llvm_type.TypeKind() {
		case llvm.FloatTypeKind:
			lv = v.compiler.builder.CreateFPTrunc(lv, llvm_type, "")
			return v.compiler.NewLLVMValue(lv, dst_typ)
		case llvm.IntegerTypeKind:
			if signed(dst_typ) {
				lv = v.compiler.builder.CreateFPToSI(lv, llvm_type, "")
			} else {
				lv = v.compiler.builder.CreateFPToUI(lv, llvm_type, "")
			}
			return v.compiler.NewLLVMValue(lv, dst_typ)
		}
	case llvm.FloatTypeKind:
		switch llvm_type.TypeKind() {
		case llvm.DoubleTypeKind:
			lv = v.compiler.builder.CreateFPExt(lv, llvm_type, "")
			return v.compiler.NewLLVMValue(lv, dst_typ)
		case llvm.IntegerTypeKind:
			if signed(dst_typ) {
				lv = v.compiler.builder.CreateFPToSI(lv, llvm_type, "")
			} else {
				lv = v.compiler.builder.CreateFPToUI(lv, llvm_type, "")
			}
			return v.compiler.NewLLVMValue(lv, dst_typ)
		}
	}
	//bitcast_value := v.compiler.builder.CreateBitCast(lv, llvm_type, "")

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
	panic(fmt.Sprint("unimplemented conversion: ", v.typ, " -> ", orig_dst_typ))
}

func (v *LLVMValue) LLVMValue() llvm.Value {
	if v.pointer != nil {
		return v.compiler.builder.CreateLoad(v.pointer.LLVMValue(), "")
	}
	return v.value
}

func (v *LLVMValue) Type() types.Type {
	return v.typ
}

func (v *LLVMValue) makePointee() *LLVMValue {
	t := v.compiler.NewLLVMValue(llvm.Value{}, types.Deref(v.typ))
	t.pointer = v
	return t
}

///////////////////////////////////////////////////////////////////////////////
// ConstValue methods.

func (lhs ConstValue) BinaryOp(op token.Token, rhs_ Value) Value {
	switch rhs := rhs_.(type) {
	case *LLVMValue:
		// Cast untyped lhs to rhs type.
		if _, ok := lhs.typ.(*types.Basic); ok {
			lhs = lhs.Convert(rhs.Type()).(ConstValue)
		}
		lhs_ := rhs.compiler.NewLLVMValue(lhs.LLVMValue(), lhs.Type())
		return lhs_.BinaryOp(op, rhs)

	case ConstValue:
		// TODO Check if either one is untyped, and convert to the
		// other's type.
		// TODO use type from typechecking here.
		c := lhs.compiler
		var typ types.Type = lhs.typ

		switch op {
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			typ = types.Bool
			// TODO deduce type from other types of operations.
			// XXX or are they always the same type as the operands?
		}

		a, b := lhs.Const.Match(rhs.Const)
		return ConstValue{a.BinaryOp(op, b), c, typ}
	}
	panic("unimplemented")
}

func (v ConstValue) UnaryOp(op token.Token) Value {
	return ConstValue{v.Const.UnaryOp(op), v.compiler, v.typ}
}

func (v ConstValue) Convert(dstTyp types.Type) Value {
	// Get the underlying type, if any.
	origDstTyp := dstTyp
	dstTyp = types.Underlying(dstTyp)

	if !types.Identical(v.typ, dstTyp) {
		isBasic := false
		if name, isname := types.Underlying(dstTyp).(*types.Name); isname {
			_, isBasic = name.Underlying.(*types.Basic)
		}

		compiler := v.compiler
		if isBasic {
			return ConstValue{v.Const.Convert(&dstTyp), compiler, origDstTyp}
		} else {
			return compiler.NewLLVMValue(v.LLVMValue(), v.Type()).Convert(origDstTyp)
			//panic(fmt.Errorf("unhandled conversion from %v to %v", v.typ, dstTyp))
		}
	} else {
		v.typ = origDstTyp
	}
	return v
}

func (v ConstValue) LLVMValue() llvm.Value {
	typ := types.Underlying(v.Type())
	if name, ok := typ.(*types.Name); ok {
		typ = name.Underlying
	}

	switch typ.(*types.Basic).Kind {
	case types.IntKind, types.UintKind:
		return llvm.ConstInt(llvm.Int32Type(), uint64(v.Int64()), true)
		// TODO 32/64bit (probably wait for gc)
		//int_val := v.Val.(*big.Int)
		//if int_val.Cmp(maxBigInt32) > 0 || int_val.Cmp(minBigInt32) < 0 {
		//	panic(fmt.Sprint("const ", int_val, " overflows int"))
		//}
		//return llvm.ConstInt(v.compiler.target.IntPtrType(), uint64(v.Int64()), true)

	case types.Int8Kind:
		return llvm.ConstInt(llvm.Int8Type(), uint64(v.Int64()), true)
	case types.Uint8Kind:
		return llvm.ConstInt(llvm.Int8Type(), uint64(v.Int64()), false)

	case types.Int16Kind:
		return llvm.ConstInt(llvm.Int16Type(), uint64(v.Int64()), true)
	case types.Uint16Kind:
		return llvm.ConstInt(llvm.Int16Type(), uint64(v.Int64()), false)

	case types.Int32Kind:
		return llvm.ConstInt(llvm.Int32Type(), uint64(v.Int64()), true)
	case types.Uint32Kind:
		return llvm.ConstInt(llvm.Int32Type(), uint64(v.Int64()), false)

	case types.Int64Kind:
		return llvm.ConstInt(llvm.Int64Type(), uint64(v.Int64()), true)
	case types.Uint64Kind:
		return llvm.ConstInt(llvm.Int64Type(), uint64(v.Int64()), false)

	case types.Float32Kind:
		return llvm.ConstFloat(llvm.FloatType(), float64(v.Float64()))
	case types.Float64Kind:
		return llvm.ConstFloat(llvm.DoubleType(), float64(v.Float64()))

	case types.UnsafePointerKind, types.UintptrKind:
		inttype := v.compiler.target.IntPtrType()
		return llvm.ConstInt(inttype, uint64(v.Int64()), false)

	case types.StringKind:
		strval := (v.Val).(string)
		strlen := len(strval)
		i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
		var ptr llvm.Value
		if strlen > 0 {
			ptr = v.compiler.builder.CreateGlobalStringPtr(strval, "")
			ptr = llvm.ConstBitCast(ptr, i8ptr)
		} else {
			ptr = llvm.ConstNull(i8ptr)
		}
		len_ := llvm.ConstInt(llvm.Int32Type(), uint64(strlen), false)
		return llvm.ConstStruct([]llvm.Value{ptr, len_}, false)

	case types.BoolKind:
		if v := v.Val.(bool); v {
			return llvm.ConstAllOnes(llvm.Int1Type())
		}
		return llvm.ConstNull(llvm.Int1Type())
	}
	panic(fmt.Errorf("Unhandled type: %v", typ)) //v.typ.Kind))
}

func (v ConstValue) Type() types.Type {
	// From the language spec:
	//   If the type is absent and the corresponding expression evaluates to
	//   an untyped constant, the type of the declared variable is bool, int,
	//   float64, or string respectively, depending on whether the value is
	//   a boolean, integer, floating-point, or string constant.
	if _, ok := v.typ.(*types.Basic); ok {
		switch v.typ {
		case types.Int.Underlying:
			v.typ = types.Int
		case types.Float64.Underlying:
			v.typ = types.Float64
		case types.Complex128.Underlying:
			v.typ = types.Complex128
		case types.Rune.Underlying:
			v.typ = types.Rune
		case types.String.Underlying:
			v.typ = types.String
		case types.Bool.Underlying:
			v.typ = types.Bool
		}
	}
	return v.typ
}

func (v ConstValue) Int64() int64 {
	return v.Val.(*big.Int).Int64()
}

func (v ConstValue) Float64() float64 {
	r := v.Val.(*big.Rat)
	return float64(r.Num().Int64()) / float64(r.Denom().Int64())
}

///////////////////////////////////////////////////////////////////////////////
// TypeValue

func (TypeValue) BinaryOp(op token.Token, rhs Value) Value { panic("this should not be called") }
func (TypeValue) UnaryOp(op token.Token) Value             { panic("this should not be called") }
func (TypeValue) Convert(typ types.Type) Value             { panic("this should not be called") }
func (TypeValue) LLVMValue() llvm.Value                    { panic("this should not be called") }
func (t TypeValue) Type() types.Type                       { return t.typ }

///////////////////////////////////////////////////////////////////////////////
// NilValue

func (NilValue) BinaryOp(op token.Token, rhs Value) Value { panic("this should not be called") }
func (NilValue) UnaryOp(op token.Token) Value             { panic("this should not be called") }
func (NilValue) LLVMValue() llvm.Value                    { panic("this should not be called") }
func (NilValue) Type() types.Type                         { panic("this should not be called") }
func (n NilValue) Convert(typ types.Type) Value {
	// TODO handle basic types specially, generating ConstValue's.
	zero := llvm.ConstNull(n.compiler.types.ToLLVM(typ))
	return n.compiler.NewLLVMValue(zero, typ)
}

// vim: set ft=go :
