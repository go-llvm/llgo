// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
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
		f := c.RuntimeFunction("runtime.chancap", "func(c uintptr) int")
		v = c.builder.CreateCall(f, []llvm.Value{arg.LLVMValue()}, "")
	}
	return c.NewValue(v, types.Typ[types.Int])
}

func (c *compiler) callLen(arg *LLVMValue) *LLVMValue {
	var lenvalue llvm.Value
	switch typ := arg.Type().Underlying().(type) {
	case *types.Array:
		lenvalue = llvm.ConstInt(c.llvmtypes.inttype, uint64(typ.Len()), false)
	case *types.Pointer:
		atyp := typ.Elem().Underlying().(*types.Array)
		lenvalue = llvm.ConstInt(c.llvmtypes.inttype, uint64(atyp.Len()), false)
	case *types.Slice:
		lenvalue = c.builder.CreateExtractValue(arg.LLVMValue(), 1, "")
	case *types.Map:
		f := c.RuntimeFunction("runtime.maplen", "func(m uintptr) int")
		lenvalue = c.builder.CreateCall(f, []llvm.Value{arg.LLVMValue()}, "")
	case *types.Basic:
		if isString(typ) {
			lenvalue = c.builder.CreateExtractValue(arg.LLVMValue(), 1, "")
		}
	case *types.Chan:
		f := c.RuntimeFunction("runtime.chanlen", "func(c uintptr) int")
		lenvalue = c.builder.CreateCall(f, []llvm.Value{arg.LLVMValue()}, "")
	}
	return c.NewValue(lenvalue, types.Typ[types.Int])
}
