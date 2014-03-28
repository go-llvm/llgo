// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/go-llvm/llvm"
)

func (c *compiler) callCap(arg *LLVMValue) *LLVMValue {
	var v llvm.Value
	switch typ := arg.Type().Underlying().(type) {
	case *types.Array:
		v = llvm.ConstInt(c.llvmtypes.inttype, uint64(typ.Len()), false)
	case *types.Pointer:
		atyp := typ.Elem().Underlying().(*types.Array)
		v = llvm.ConstInt(c.llvmtypes.inttype, uint64(atyp.Len()), false)
	case *types.Slice:
		v = c.builder.CreateExtractValue(arg.LLVMValue(), 2, "")
	case *types.Chan:
		f := c.runtime.chancap.LLVMValue()
		v = c.builder.CreateCall(f, []llvm.Value{arg.LLVMValue()}, "")
	}
	return c.NewValue(v, types.Typ[types.Int])
}

func (fr *frame) callLen(arg *LLVMValue) *LLVMValue {
	var lenvalue llvm.Value
	switch typ := arg.Type().Underlying().(type) {
	case *types.Array:
		lenvalue = llvm.ConstInt(fr.llvmtypes.inttype, uint64(typ.Len()), false)
	case *types.Pointer:
		atyp := typ.Elem().Underlying().(*types.Array)
		lenvalue = llvm.ConstInt(fr.llvmtypes.inttype, uint64(atyp.Len()), false)
	case *types.Slice:
		lenvalue = fr.builder.CreateExtractValue(arg.LLVMValue(), 1, "")
	case *types.Map:
		lenvalue = fr.runtime.mapLen.call(fr, arg.LLVMValue())[0]
	case *types.Basic:
		if isString(typ) {
			lenvalue = fr.builder.CreateExtractValue(arg.LLVMValue(), 1, "")
		}
	case *types.Chan:
		f := fr.runtime.chanlen.LLVMValue()
		lenvalue = fr.builder.CreateCall(f, []llvm.Value{arg.LLVMValue()}, "")
	}
	return fr.NewValue(lenvalue, types.Typ[types.Int])
}

// callAppend takes two slices of the same type, and yields
// the result of appending the second to the first.
func (fr *frame) callAppend(a, b *LLVMValue) *LLVMValue {
	bptr := fr.builder.CreateExtractValue(b.LLVMValue(), 0, "")
	blen := fr.builder.CreateExtractValue(b.LLVMValue(), 1, "")
	elemsizeInt64 := fr.types.Sizeof(a.Type().Underlying().(*types.Slice).Elem())
	elemsize := llvm.ConstInt(fr.target.IntPtrType(), uint64(elemsizeInt64), false)
	result := fr.runtime.append.call(fr, a.LLVMValue(), bptr, blen, elemsize)[0]
	return fr.NewValue(result, a.Type())
}

// callCopy takes two slices a and b of the same type, and
// yields the result of calling "copy(a, b)".
func (fr *frame) callCopy(dest, source *LLVMValue) *LLVMValue {
	aptr := fr.builder.CreateExtractValue(dest.LLVMValue(), 0, "")
	alen := fr.builder.CreateExtractValue(dest.LLVMValue(), 1, "")
	bptr := fr.builder.CreateExtractValue(source.LLVMValue(), 0, "")
	blen := fr.builder.CreateExtractValue(source.LLVMValue(), 1, "")
	aless := fr.builder.CreateICmp(llvm.IntULT, alen, blen, "")
	minlen := fr.builder.CreateSelect(aless, alen, blen, "")
	elemsizeInt64 := fr.types.Sizeof(dest.Type().Underlying().(*types.Slice).Elem())
	elemsize := llvm.ConstInt(fr.types.inttype, uint64(elemsizeInt64), false)
	bytes := fr.builder.CreateMul(minlen, elemsize, "")
	fr.runtime.copy.call(fr, aptr, bptr, bytes)
	return fr.NewValue(minlen, types.Typ[types.Int])
}
