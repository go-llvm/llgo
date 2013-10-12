// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

func (c *compiler) callCap(arg *LLVMValue) *LLVMValue {
	var capvalue llvm.Value
	switch typ := arg.Type().Underlying().(type) {
	case *types.Array:
		capvalue = llvm.ConstInt(c.llvmtypes.inttype, uint64(typ.Len()), false)
	case *types.Pointer:
		atyp := typ.Elem().Underlying().(*types.Array)
		capvalue = llvm.ConstInt(c.llvmtypes.inttype, uint64(atyp.Len()), false)
	case *types.Slice:
		sliceval := arg.LLVMValue()
		capvalue = c.builder.CreateExtractValue(sliceval, 2, "")
	case *types.Chan:
		chanval := arg.LLVMValue()
		f := c.RuntimeFunction("runtime.chancap", "func(c uintptr) int")
		capvalue = c.builder.CreateCall(f, []llvm.Value{chanval}, "")
	}
	return c.NewValue(capvalue, types.Typ[types.Int])
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
		sliceval := arg.LLVMValue()
		lenvalue = c.builder.CreateExtractValue(sliceval, 1, "")
	case *types.Map:
		mapval := arg.LLVMValue()
		f := c.RuntimeFunction("runtime.maplen", "func(m uintptr) int")
		lenvalue = c.builder.CreateCall(f, []llvm.Value{mapval}, "")
	case *types.Basic:
		if isString(typ) {
			lenvalue = c.builder.CreateExtractValue(arg.LLVMValue(), 1, "")
		}
	case *types.Chan:
		chanval := arg.LLVMValue()
		f := c.RuntimeFunction("runtime.chanlen", "func(c uintptr) int")
		lenvalue = c.builder.CreateCall(f, []llvm.Value{chanval}, "")
	}
	return c.NewValue(lenvalue, types.Typ[types.Int])
}
