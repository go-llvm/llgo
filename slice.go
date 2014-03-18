// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// makeSlice allocates a new slice with the optional length and capacity,
// initialising its contents to their zero values.
func (fr *frame) makeSlice(sliceType types.Type, length, capacity *LLVMValue) *LLVMValue {
	length = fr.convert(length, types.Typ[types.Int]).(*LLVMValue)
	capacity = fr.convert(capacity, types.Typ[types.Int]).(*LLVMValue)
	makeslice := fr.runtime.makeslice.LLVMValue()
	runtimeType := fr.builder.CreatePtrToInt(fr.types.ToRuntime(sliceType), fr.target.IntPtrType(), "")
	llslice := fr.builder.CreateCall(makeslice, []llvm.Value{
		runtimeType, length.LLVMValue(), capacity.LLVMValue(),
	}, "")
	llslice = fr.coerceSlice(llslice, fr.types.ToLLVM(sliceType))
	return fr.NewValue(llslice, sliceType)
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

func (fr *frame) slice(x, low, high *LLVMValue) *LLVMValue {
	if low != nil {
		low = fr.convert(low, types.Typ[types.Int]).(*LLVMValue)
	} else {
		low = fr.NewValue(llvm.ConstNull(fr.types.inttype), types.Typ[types.Int])
	}

	if high != nil {
		high = fr.convert(high, types.Typ[types.Int]).(*LLVMValue)
	} else {
		// all bits set is -1
		high = fr.NewValue(llvm.ConstAllOnes(fr.types.inttype), types.Typ[types.Int])
	}

	switch typ := x.Type().Underlying().(type) {
	case *types.Pointer: // *array
		sliceslice := fr.runtime.sliceslice.LLVMValue()
		i8slice := sliceslice.Type().ElementType().ReturnType()
		sliceValue := llvm.Undef(i8slice) // temporary slice
		arraytyp := typ.Elem().Underlying().(*types.Array)
		arrayptr := x.LLVMValue()
		arrayptr = fr.builder.CreateBitCast(arrayptr, i8slice.StructElementTypes()[0], "")
		arraylen := llvm.ConstInt(fr.llvmtypes.inttype, uint64(arraytyp.Len()), false)
		sliceValue = fr.builder.CreateInsertValue(sliceValue, arrayptr, 0, "")
		sliceValue = fr.builder.CreateInsertValue(sliceValue, arraylen, 1, "")
		sliceValue = fr.builder.CreateInsertValue(sliceValue, arraylen, 2, "")
		sliceTyp := types.NewSlice(arraytyp.Elem())
		runtimeTyp := fr.types.ToRuntime(sliceTyp)
		runtimeTyp = fr.builder.CreatePtrToInt(runtimeTyp, fr.target.IntPtrType(), "")
		args := []llvm.Value{runtimeTyp, sliceValue, low.LLVMValue(), high.LLVMValue()}
		result := fr.builder.CreateCall(sliceslice, args, "")
		llvmSliceTyp := fr.types.ToLLVM(sliceTyp)
		return fr.NewValue(fr.coerceSlice(result, llvmSliceTyp), sliceTyp)
	case *types.Slice:
		sliceslice := fr.runtime.sliceslice.LLVMValue()
		i8slice := sliceslice.Type().ElementType().ReturnType()
		sliceValue := x.LLVMValue()
		sliceTyp := sliceValue.Type()
		sliceValue = fr.coerceSlice(sliceValue, i8slice)
		runtimeTyp := fr.types.ToRuntime(x.Type())
		runtimeTyp = fr.builder.CreatePtrToInt(runtimeTyp, fr.target.IntPtrType(), "")
		args := []llvm.Value{runtimeTyp, sliceValue, low.LLVMValue(), high.LLVMValue()}
		result := fr.builder.CreateCall(sliceslice, args, "")
		return fr.NewValue(fr.coerceSlice(result, sliceTyp), x.Type())
	case *types.Basic:
		stringslice := fr.runtime.stringslice.LLVMValue()
		llv := x.LLVMValue()
		args := []llvm.Value{
			fr.coerceString(llv, stringslice.Type().ElementType().ParamTypes()[0]),
			low.LLVMValue(),
			high.LLVMValue(),
		}
		result := fr.builder.CreateCall(stringslice, args, "")
		return fr.NewValue(fr.coerceString(result, llv.Type()), x.Type())
	default:
		panic("unimplemented")
	}
	panic("unreachable")
}
