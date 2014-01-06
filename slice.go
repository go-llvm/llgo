// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// makeLiteralSlice allocates a new slice, storing in it the provided elements.
func (c *compiler) makeLiteralSlice(v []llvm.Value, elttyp types.Type) llvm.Value {
	n := llvm.ConstInt(c.types.inttype, uint64(len(v)), false)
	eltType := c.types.ToLLVM(elttyp)
	arrayType := llvm.ArrayType(eltType, len(v))
	mem := c.createMalloc(llvm.SizeOf(arrayType))
	mem = c.builder.CreateIntToPtr(mem, llvm.PointerType(eltType, 0), "")
	for i, value := range v {
		indices := []llvm.Value{llvm.ConstInt(llvm.Int32Type(), uint64(i), false)}
		ep := c.builder.CreateGEP(mem, indices, "")
		c.builder.CreateStore(value, ep)
	}
	slicetyp := types.NewSlice(elttyp)
	struct_ := llvm.Undef(c.types.ToLLVM(slicetyp))
	struct_ = c.builder.CreateInsertValue(struct_, mem, 0, "")
	struct_ = c.builder.CreateInsertValue(struct_, n, 1, "")
	struct_ = c.builder.CreateInsertValue(struct_, n, 2, "")
	return struct_
}

// makeSlice allocates a new slice with the optional length and capacity,
// initialising its contents to their zero values.
func (c *compiler) makeSlice(sliceType types.Type, length, capacity *LLVMValue) *LLVMValue {
	length = length.Convert(types.Typ[types.Int]).(*LLVMValue)
	capacity = capacity.Convert(types.Typ[types.Int]).(*LLVMValue)
	makeslice := c.runtime.makeslice.LLVMValue()
	runtimeType := c.builder.CreatePtrToInt(c.types.ToRuntime(sliceType), c.target.IntPtrType(), "")
	llslice := c.builder.CreateCall(makeslice, []llvm.Value{
		runtimeType, length.LLVMValue(), capacity.LLVMValue(),
	}, "")
	llslice = c.coerceSlice(llslice, c.types.ToLLVM(sliceType))
	return c.NewValue(llslice, sliceType)
}

// coerceSlice takes a slice of one element type and coerces it to a
// slice of another.
func (c *compiler) coerceSlice(src llvm.Value, dsttyp llvm.Type) llvm.Value {
	dst := llvm.Undef(dsttyp)
	srcmem := c.builder.CreateExtractValue(src, 0, "")
	srclen := c.builder.CreateExtractValue(src, 1, "")
	srccap := c.builder.CreateExtractValue(src, 2, "")
	dstmemtyp := dsttyp.StructElementTypes()[0]
	dstmem := c.builder.CreateBitCast(srcmem, dstmemtyp, "")
	dst = c.builder.CreateInsertValue(dst, dstmem, 0, "")
	dst = c.builder.CreateInsertValue(dst, srclen, 1, "")
	dst = c.builder.CreateInsertValue(dst, srccap, 2, "")
	return dst
}

func (c *compiler) slice(x, low, high *LLVMValue) *LLVMValue {
	if low != nil {
		low = low.Convert(types.Typ[types.Int]).(*LLVMValue)
	} else {
		low = c.NewValue(llvm.ConstNull(c.types.inttype), types.Typ[types.Int])
	}

	if high != nil {
		high = high.Convert(types.Typ[types.Int]).(*LLVMValue)
	} else {
		// all bits set is -1
		high = c.NewValue(llvm.ConstAllOnes(c.types.inttype), types.Typ[types.Int])
	}

	switch typ := x.Type().Underlying().(type) {
	case *types.Pointer: // *array
		sliceslice := c.runtime.sliceslice.LLVMValue()
		i8slice := sliceslice.Type().ElementType().ReturnType()
		sliceValue := llvm.Undef(i8slice) // temporary slice
		arraytyp := typ.Elem().Underlying().(*types.Array)
		arrayptr := x.LLVMValue()
		arrayptr = c.builder.CreateBitCast(arrayptr, i8slice.StructElementTypes()[0], "")
		arraylen := llvm.ConstInt(c.llvmtypes.inttype, uint64(arraytyp.Len()), false)
		sliceValue = c.builder.CreateInsertValue(sliceValue, arrayptr, 0, "")
		sliceValue = c.builder.CreateInsertValue(sliceValue, arraylen, 1, "")
		sliceValue = c.builder.CreateInsertValue(sliceValue, arraylen, 2, "")
		sliceTyp := types.NewSlice(arraytyp.Elem())
		runtimeTyp := c.types.ToRuntime(sliceTyp)
		runtimeTyp = c.builder.CreatePtrToInt(runtimeTyp, c.target.IntPtrType(), "")
		args := []llvm.Value{runtimeTyp, sliceValue, low.LLVMValue(), high.LLVMValue()}
		result := c.builder.CreateCall(sliceslice, args, "")
		llvmSliceTyp := c.types.ToLLVM(sliceTyp)
		return c.NewValue(c.coerceSlice(result, llvmSliceTyp), sliceTyp)
	case *types.Slice:
		sliceslice := c.runtime.sliceslice.LLVMValue()
		i8slice := sliceslice.Type().ElementType().ReturnType()
		sliceValue := x.LLVMValue()
		sliceTyp := sliceValue.Type()
		sliceValue = c.coerceSlice(sliceValue, i8slice)
		runtimeTyp := c.types.ToRuntime(x.Type())
		runtimeTyp = c.builder.CreatePtrToInt(runtimeTyp, c.target.IntPtrType(), "")
		args := []llvm.Value{runtimeTyp, sliceValue, low.LLVMValue(), high.LLVMValue()}
		result := c.builder.CreateCall(sliceslice, args, "")
		return c.NewValue(c.coerceSlice(result, sliceTyp), x.Type())
	case *types.Basic:
		stringslice := c.runtime.stringslice.LLVMValue()
		llv := x.LLVMValue()
		args := []llvm.Value{
			c.coerceString(llv, stringslice.Type().ElementType().ParamTypes()[0]),
			low.LLVMValue(),
			high.LLVMValue(),
		}
		result := c.builder.CreateCall(stringslice, args, "")
		return c.NewValue(c.coerceString(result, llv.Type()), x.Type())
	default:
		panic("unimplemented")
	}
	panic("unreachable")
}
