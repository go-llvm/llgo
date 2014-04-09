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

func (fr *frame) slice(x, low, high *govalue, slicetyp types.Type) *govalue {
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
		errcode = gccgoRuntimeErrorARRAY_SLICE_OUT_OF_BOUNDS
		arraytyp := typ.Elem().Underlying().(*types.Array)
		elemtyp = arraytyp.Elem()
		arrayptr = x.value
		arrayptr = fr.builder.CreateBitCast(arrayptr, llvm.PointerType(llvm.Int8Type(), 0), "")
		arraylen = llvm.ConstInt(fr.llvmtypes.inttype, uint64(arraytyp.Len()), false)
		arraycap = arraylen
	case *types.Slice:
		errcode = gccgoRuntimeErrorSLICE_SLICE_OUT_OF_BOUNDS
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

	fr.condBrRuntimeError(cond, errcode)

	slicelen := fr.builder.CreateSub(highval, lowval, "")
	slicecap := fr.builder.CreateSub(arraycap, lowval, "")

	elemsize := llvm.ConstInt(fr.llvmtypes.inttype, uint64(fr.llvmtypes.Sizeof(elemtyp)), false)
	offset := fr.builder.CreateMul(lowval, elemsize, "")

	sliceptr := fr.builder.CreateInBoundsGEP(arrayptr, []llvm.Value{offset}, "")

	llslicetyp := fr.llvmtypes.sliceBackendType().ToLLVM(fr.llvmtypes.ctx)
	sliceValue := llvm.Undef(llslicetyp)
	sliceValue = fr.builder.CreateInsertValue(sliceValue, sliceptr, 0, "")
	sliceValue = fr.builder.CreateInsertValue(sliceValue, slicelen, 1, "")
	sliceValue = fr.builder.CreateInsertValue(sliceValue, slicecap, 2, "")

	return newValue(sliceValue, slicetyp)
}
