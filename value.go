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

// LLVMValue represents a dynamic value produced as the result of an
// expression.
type LLVMValue struct {
	compiler *compiler
	value    llvm.Value
	typ      types.Type
	indirect bool
	address  *LLVMValue // Value that dereferenced to this value.
	receiver *LLVMValue
}

// ConstValue represents a constant value produced as the result of an
// expression.
type ConstValue struct {
	types.Const
	compiler *compiler
	typ      *types.Basic
}

// TypeValue represents a Type result of an expression. All methods
// other than Type() will panic when called.
type TypeValue struct {
	typ types.Type
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
		typ = types.Rune.Underlying.(*types.Basic)
	case token.STRING:
		typ = types.String.Underlying.(*types.Basic)
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
	c := lhs.compiler
	b := lhs.compiler.builder

	// Later we can do better by treating constants specially. For now, let's
	// convert to LLVMValue's.
	var rhs *LLVMValue
	switch rhs_ := rhs_.(type) {
	case *LLVMValue:
		// Deref rhs, if it's indirect.
		if rhs_.indirect {
			rhs = rhs_.Deref()
		} else {
			rhs = rhs_
		}
	case ConstValue:
		switch rhs_.typ.Kind {
		case types.NilKind:
			rhs = rhs_.Convert(lhs.Type()).(*LLVMValue)
		case types.UntypedIntKind, types.UntypedFloatKind, types.UntypedComplexKind:
			rhs_ = rhs_.Convert(lhs.Type()).(ConstValue)
			fallthrough
		default:
			rhs = c.NewLLVMValue(rhs_.LLVMValue(), rhs_.Type())
		}
	}

	// Special case for structs.
	// TODO handle strings as an even more special case.
	if struct_type, ok := types.Underlying(lhs.typ).(*types.Struct); ok {
		// TODO check types are the same.

		element_types_count := lhs.value.Type().StructElementTypesCount()
		struct_fields := struct_type.Fields

		if element_types_count > 0 {
			t := c.ObjGetType(struct_fields[0])
			first_lhs := c.NewLLVMValue(b.CreateExtractValue(lhs.value, 0, ""), t)
			first_rhs := c.NewLLVMValue(b.CreateExtractValue(rhs.value, 0, ""), t)
			first := first_lhs.BinaryOp(op, first_rhs)

			logical_op := token.LAND
			if op == token.NEQ {
				logical_op = token.LOR
			}

			result := first
			for i := 1; i < element_types_count; i++ {
				t := c.ObjGetType(struct_fields[i])
				next_lhs := c.NewLLVMValue(b.CreateExtractValue(lhs.value, i, ""), t)
				next_rhs := c.NewLLVMValue(b.CreateExtractValue(rhs.value, i, ""), t)
				next := next_lhs.BinaryOp(op, next_rhs)
				result = result.BinaryOp(logical_op, next)
			}
			return result
		}
	}

	// Interfaces.
	if _, ok := types.Underlying(lhs.typ).(*types.Interface); ok {
		// TODO check for interface/interface comparison vs. interface/value comparison.

		// nil comparison
		if /*rhs.value.IsConstant() &&*/ rhs.value.IsNull() {
			var result llvm.Value
			if op == token.EQL {
				valueNull := b.CreateIsNull(b.CreateExtractValue(lhs.value, 0, ""), "")
				typeNull := b.CreateIsNull(b.CreateExtractValue(lhs.value, 1, ""), "")
				result = b.CreateAnd(typeNull, valueNull, "")
			} else {
				valueNotNull := b.CreateIsNotNull(b.CreateExtractValue(lhs.value, 0, ""), "")
				typeNotNull := b.CreateIsNotNull(b.CreateExtractValue(lhs.value, 1, ""), "")
				result = b.CreateOr(typeNotNull, valueNotNull, "")
			}
			return c.NewLLVMValue(result, types.Bool)
		}

		// First, check that the dynamic types are identical.
		// FIXME provide runtime function for type identity comparison, and
		// value comparisons.
		lhsType := b.CreateExtractValue(lhs.value, 1, "")
		rhsType := b.CreateExtractValue(rhs.value, 1, "")
		diff := b.CreatePtrDiff(lhsType, rhsType, "")
		zero := llvm.ConstNull(diff.Type())

		var result llvm.Value
		if op == token.EQL {
			typesIdentical := b.CreateICmp(llvm.IntEQ, diff, zero, "")
			//valuesEqual := ...
			//result = b.CreateAnd(typesIdentical, valuesEqual, "")
			result = typesIdentical
		} else {
			typesDifferent := b.CreateICmp(llvm.IntNE, diff, zero, "")
			//valuesUnequal := ...
			//result = b.CreateOr(typesDifferent, valuesUnequal, "")
			result = typesDifferent
		}
		return c.NewLLVMValue(result, types.Bool)
	}

	if types.Underlying(lhs.typ) == types.String.Underlying {
		if types.Underlying(rhs.typ) == types.String.Underlying {
			switch op {
			case token.ADD:
				return c.concatenateStrings(lhs, rhs)
			//case token.EQL:
			//	return c.compareStringsEqual(lhs.value, rhs.value)
			//case token.LSS:
			//	return c.compareStringsLess(lhs.value, rhs.value)
			//case token.LEQ:
			//	return c.compareStringsLessEqual(lhs.value, rhs.value)
			default:
				panic(fmt.Sprint("Unimplemented operator: ", op))
			}
		}
		panic("unimplemented")
	}

	// Determine whether to use integer or floating point instructions.
	// TODO determine the NaN rules.
	isfp := types.Identical(types.Underlying(lhs.typ), types.Float32) ||
		types.Identical(types.Underlying(lhs.typ), types.Float64)

	switch op {
	case token.MUL:
		result = b.CreateMul(lhs.value, rhs.value, "")
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	case token.QUO:
		result = b.CreateUDiv(lhs.value, rhs.value, "")
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	case token.ADD:
		result = b.CreateAdd(lhs.value, rhs.value, "")
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	case token.SUB:
		result = b.CreateSub(lhs.value, rhs.value, "")
		return lhs.compiler.NewLLVMValue(result, lhs.typ)
	case token.NEQ:
		if isfp {
			result = b.CreateFCmp(llvm.FloatONE, lhs.value, rhs.value, "")
		} else {
			result = b.CreateICmp(llvm.IntNE, lhs.value, rhs.value, "")
		}
		return lhs.compiler.NewLLVMValue(result, types.Bool)
	case token.EQL:
		result = b.CreateICmp(llvm.IntEQ, lhs.value, rhs.value, "")
		return lhs.compiler.NewLLVMValue(result, types.Bool)
	case token.LSS:
		result = b.CreateICmp(llvm.IntULT, lhs.value, rhs.value, "")
		return lhs.compiler.NewLLVMValue(result, types.Bool)
	case token.LEQ: // TODO signed/unsigned
		result = b.CreateICmp(llvm.IntULE, lhs.value, rhs.value, "")
		return lhs.compiler.NewLLVMValue(result, types.Bool)
	case token.LAND:
		result = b.CreateAnd(lhs.value, rhs.value, "")
		return lhs.compiler.NewLLVMValue(result, types.Bool)
	case token.LOR:
		result = b.CreateOr(lhs.value, rhs.value, "")
		return lhs.compiler.NewLLVMValue(result, types.Bool)
	default:
		panic(fmt.Sprint("Unimplemented operator: ", op))
	}
	panic("unreachable")
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

func (v *LLVMValue) Convert(dst_typ types.Type) Value {
	// If it's a stack allocated value, we'll want to compare the
	// value type, not the pointer type.
	src_typ := v.typ
	if v.indirect {
		src_typ = types.Deref(src_typ)
	}

	// Get the underlying type, if any.
	orig_dst_typ := dst_typ
	if name, isname := dst_typ.(*types.Name); isname {
		dst_typ = types.Underlying(name)
	}

	// Get the underlying type, if any.
	if name, isname := src_typ.(*types.Name); isname {
		src_typ = types.Underlying(name)
	}

	// Identical (underlying) types? Just swap in the destination type.
	if types.Identical(src_typ, dst_typ) {
		dst_typ = orig_dst_typ
		if v.indirect {
			dst_typ = &types.Pointer{Base: dst_typ}
		}
		// XXX do we need to copy address/receiver here?
		newv := v.compiler.NewLLVMValue(v.value, dst_typ)
		newv.indirect = true
		return newv
	}

	// Convert from an interface type.
	if _, isinterface := src_typ.(*types.Interface); isinterface {
		if interface_, isinterface := dst_typ.(*types.Interface); isinterface {
			return v.convertI2I(interface_)
		} else {
			return v.convertI2V(dst_typ)
		}
	}

	// Converting to an interface type.
	if interface_, isinterface := dst_typ.(*types.Interface); isinterface {
		return v.convertV2I(interface_)
	}

	// TODO other special conversions, e.g. int->string.

	llvm_type := v.compiler.types.ToLLVM(dst_typ)
	if v.indirect {
		v = v.Deref()
	}

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
		case types.UntypedIntKind, types.UntypedFloatKind, types.UntypedComplexKind:
			lhs = lhs.Convert(rhs.Type()).(ConstValue)
		}

		lhs_ := rhs.compiler.NewLLVMValue(lhs.LLVMValue(), lhs.Type())
		return lhs_.BinaryOp(op, rhs)

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

func (v ConstValue) Convert(dst_typ types.Type) Value {
	// Get the underlying type, if any.
	if name, isname := dst_typ.(*types.Name); isname {
		dst_typ = types.Underlying(name)
	}

	if !types.Identical(v.typ, dst_typ) {
		// Get the Basic type.
		if name, isname := dst_typ.(*types.Name); isname {
			dst_typ = name.Underlying
		}

		compiler := v.compiler
		if basic, ok := dst_typ.(*types.Basic); ok {
			return ConstValue{*v.Const.Convert(&dst_typ), compiler, basic}
		} else {
			// Special case for 'nil'
			if v.typ.Kind == types.NilKind {
				zero := llvm.ConstNull(compiler.types.ToLLVM(dst_typ))
				return compiler.NewLLVMValue(zero, dst_typ)
			}
			panic("unhandled conversion")
		}
	} else {
		// TODO convert to dst type. ConstValue may need to change to allow
		// storage of types other than Basic.
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
	case types.Int32Kind, types.Uint32Kind, types.RuneKind:
		return llvm.ConstInt(llvm.Int32Type(), uint64(v.Int64()), false)
	case types.UnsafePointerKind, types.UintptrKind:
		inttype := v.compiler.target.IntPtrType()
		return llvm.ConstInt(inttype, uint64(v.Int64()), false)
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
	panic(fmt.Errorf("Unhandled type: %v", v.typ.Kind))
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

///////////////////////////////////////////////////////////////////////////////
// 

func (TypeValue) BinaryOp(op token.Token, rhs Value) Value { panic("this should not be called") }
func (TypeValue) UnaryOp(op token.Token) Value             { panic("this should not be called") }
func (TypeValue) Convert(typ types.Type) Value             { panic("this should not be called") }
func (TypeValue) LLVMValue() llvm.Value                    { panic("this should not be called") }
func (t TypeValue) Type() types.Type                       { return t.typ }

// vim: set ft=go :
