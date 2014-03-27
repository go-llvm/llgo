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
func (c *compiler) callAppend(a, b *LLVMValue) *LLVMValue {
	f := c.runtime.sliceappend.LLVMValue()
	i8slice := f.Type().ElementType().ReturnType()
	lla := a.LLVMValue()
	llaType := lla.Type()
	runtimeType := c.types.ToRuntime(a.Type())
	args := []llvm.Value{
		c.builder.CreatePtrToInt(runtimeType, c.target.IntPtrType(), ""),
		c.coerceSlice(lla, i8slice),
		c.coerceSlice(b.LLVMValue(), i8slice),
	}
	result := c.builder.CreateCall(f, args, "")
	return c.NewValue(c.coerceSlice(result, llaType), a.Type())
}

// callCopy takes two slices a and b of the same type, and
// yields the result of calling "copy(a, b)".
func (c *compiler) callCopy(dest, source *LLVMValue) *LLVMValue {
	runtimeTyp := c.types.ToRuntime(dest.Type())
	runtimeTyp = c.builder.CreatePtrToInt(runtimeTyp, c.target.IntPtrType(), "")
	slicecopy := c.runtime.slicecopy.value
	i8slice := slicecopy.Type().ElementType().ParamTypes()[1]
	args := []llvm.Value{
		runtimeTyp,
		c.coerceSlice(dest.LLVMValue(), i8slice),
		c.coerceSlice(source.LLVMValue(), i8slice),
	}
	result := c.builder.CreateCall(slicecopy, args, "")
	return c.NewValue(result, types.Typ[types.Int])
}
