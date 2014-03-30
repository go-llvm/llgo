// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/go-llvm/llvm"
)

// makeSlice allocates a new slice with the optional length and capacity,
// initialising its contents to their zero values.
func (fr *frame) makeSlice(sliceType types.Type, length, capacity *govalue) *govalue {
	length = fr.convert(length, types.Typ[types.Uintptr])
	capacity = fr.convert(capacity, types.Typ[types.Uintptr])
	runtimeType := fr.types.ToRuntime(sliceType)
	llslice := fr.runtime.makeSlice.call(fr, runtimeType, length.value, capacity.value)
	return newValue(llslice[0], sliceType)
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

func (fr *frame) slice(x, low, high *govalue) *govalue {
	var lowval, highval llvm.Value
	if low != nil {
		lowval = fr.convert(low, types.Typ[types.Int]).value
	} else {
		lowval = llvm.ConstNull(fr.types.inttype)
	}
	if high != nil {
		highval = fr.convert(high, types.Typ[types.Int]).value
	}

	var arrayptr, arraylen, arraycap llvm.Value
	var elemtyp types.Type
	var errcode uint64
	switch typ := x.Type().Underlying().(type) {
	case *types.Pointer: // *array
		errcode = 3 // TODO(pcc): symbolic
		arraytyp := typ.Elem().Underlying().(*types.Array)
		elemtyp = arraytyp.Elem()
		arrayptr = x.value
		arrayptr = fr.builder.CreateBitCast(arrayptr, llvm.PointerType(llvm.Int8Type(), 0), "")
		arraylen = llvm.ConstInt(fr.llvmtypes.inttype, uint64(arraytyp.Len()), false)
		arraycap = arraylen
	case *types.Slice:
		errcode = 4 // TODO(pcc): symbolic
		elemtyp = typ.Elem()
		sliceValue := x.value
		arrayptr = fr.builder.CreateExtractValue(sliceValue, 0, "")
		arraylen = fr.builder.CreateExtractValue(sliceValue, 1, "")
		arraycap = fr.builder.CreateExtractValue(sliceValue, 2, "")
	case *types.Basic:
		if high == nil {
			highval = llvm.ConstAllOnes(fr.types.inttype) // -1
		}
		result := fr.runtime.stringSlice.call(fr, x.value, lowval, highval)
		return newValue(result[0], x.Type())
	default:
		panic("unimplemented")
	}
	if high == nil {
		highval = arraylen
	}

	// Bounds checking: low >= 0, low <= high <= cap
	zero := llvm.ConstNull(fr.types.inttype)
	l0 := fr.builder.CreateICmp(llvm.IntSLT, lowval, zero, "")
	zh := fr.builder.CreateICmp(llvm.IntSLT, arraycap, highval, "")
	hl := fr.builder.CreateICmp(llvm.IntSLT, highval, lowval, "")

	cond := fr.builder.CreateOr(l0, zh, "")
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

	return newValue(sliceValue, types.NewSlice(elemtyp))
}
