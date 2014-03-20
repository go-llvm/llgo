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
	length = fr.convert(length, types.Typ[types.Uintptr]).(*LLVMValue)
	capacity = fr.convert(capacity, types.Typ[types.Uintptr]).(*LLVMValue)
	runtimeType := fr.builder.CreateBitCast(fr.types.getTypeDescriptorPointer(sliceType), llvm.PointerType(llvm.Int8Type(), 0), "")
	llslice := fr.runtime.makeSlice.call(fr, runtimeType, length.LLVMValue(), capacity.LLVMValue())
	return fr.NewValue(llslice[0], sliceType)
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
	var arrayptr, arraylen, arraycap llvm.Value
	var elemtyp types.Type
	var errcode uint64
	switch typ := x.Type().Underlying().(type) {
	case *types.Pointer: // *array
		errcode = 3 // TODO(pcc): symbolic
		arraytyp := typ.Elem().Underlying().(*types.Array)
		elemtyp = arraytyp.Elem()
		arrayptr = x.LLVMValue()
		arrayptr = fr.builder.CreateBitCast(arrayptr, llvm.PointerType(llvm.Int8Type(), 0), "")
		arraylen = llvm.ConstInt(fr.llvmtypes.inttype, uint64(arraytyp.Len()), false)
		arraycap = arraylen
	case *types.Slice:
		errcode = 4 // TODO(pcc): symbolic
		elemtyp = typ.Elem()
		sliceValue := x.LLVMValue()
		arrayptr = fr.builder.CreateExtractValue(sliceValue, 0, "")
		arraylen = fr.builder.CreateExtractValue(sliceValue, 1, "")
		arraycap = fr.builder.CreateExtractValue(sliceValue, 2, "")
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

	var lowval, highval llvm.Value
	if low != nil {
		lowval = fr.convert(low, types.Typ[types.Int]).LLVMValue()
	} else {
		lowval = llvm.ConstNull(fr.types.inttype)
	}

	if high != nil {
		highval = fr.convert(high, types.Typ[types.Int]).LLVMValue()
	} else {
		highval = arraylen
	}

	// Bounds checking: low >= 0, high >= 0, low <= high <= size
	zero := llvm.ConstNull(fr.types.inttype)
	l0 := fr.builder.CreateICmp(llvm.IntSLT, lowval, zero, "")
	h0 := fr.builder.CreateICmp(llvm.IntSLT, highval, zero, "")
	zh := fr.builder.CreateICmp(llvm.IntSLT, arraycap, highval, "")
	hl := fr.builder.CreateICmp(llvm.IntSLT, highval, lowval, "")

	cond := fr.builder.CreateOr(l0, h0, "")
	cond = fr.builder.CreateOr(cond, zh, "")
	cond = fr.builder.CreateOr(cond, hl, "")

	errorbb := llvm.AddBasicBlock(fr.function, "")
	contbb := llvm.AddBasicBlock(fr.function, "")
	fr.builder.CreateCondBr(cond, errorbb, contbb)

	fr.builder.SetInsertPointAtEnd(errorbb)
	fr.runtime.runtimeError.call(fr, llvm.ConstInt(llvm.Int32Type(), errcode, false))
	fr.builder.CreateUnreachable()

	fr.builder.SetInsertPointAtEnd(contbb)

	slicelen := fr.builder.CreateSub(highval, lowval, "")
	slicecap := fr.builder.CreateSub(arraycap, lowval, "")

	elemsize := llvm.ConstInt(fr.llvmtypes.inttype, uint64(fr.llvmtypes.Sizeof(elemtyp)), false)
	offset := fr.builder.CreateMul(lowval, elemsize, "")

	sliceptr := fr.builder.CreateInBoundsGEP(arrayptr, []llvm.Value{offset}, "")

	slicetyp := fr.llvmtypes.sliceBackendType().ToLLVM(fr.llvmtypes.ctx)
	sliceValue := llvm.Undef(slicetyp)
	sliceValue = fr.builder.CreateInsertValue(sliceValue, sliceptr, 0, "")
	sliceValue = fr.builder.CreateInsertValue(sliceValue, slicelen, 1, "")
	sliceValue = fr.builder.CreateInsertValue(sliceValue, slicecap, 2, "")

	return fr.NewValue(sliceValue, types.NewSlice(elemtyp))
}
