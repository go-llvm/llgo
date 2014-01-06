// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// convertE2V converts (type asserts) an interface value to a concrete type.
func (v *LLVMValue) convertE2V(typ types.Type) (result, success *LLVMValue) {
	c := v.compiler
	f := c.runtime.convertE2V.LLVMValue()
	rtyp := c.types.ToRuntime(typ)
	stackptr := c.stacksave()
	ptr := c.builder.CreateAlloca(c.types.ToLLVM(typ), "")
	args := []llvm.Value{
		coerce(c.builder, v.LLVMValue(), c.runtime.eface.llvm),
		c.builder.CreatePtrToInt(rtyp, c.target.IntPtrType(), ""),
		c.builder.CreatePtrToInt(ptr, c.target.IntPtrType(), ""),
	}
	succ := c.builder.CreateCall(f, args, "")
	res := c.builder.CreateLoad(ptr, "")
	c.stackrestore(stackptr)
	return c.NewValue(res, typ), c.NewValue(succ, types.Typ[types.Bool])
}

// mustConvertE2V calls convertE2V, panicking if the assertion failed.
func (v *LLVMValue) mustConvertE2V(typ types.Type) *LLVMValue {
	c := v.compiler
	f := c.runtime.mustConvertE2V.LLVMValue()
	rtyp := c.types.ToRuntime(typ)
	stackptr := c.stacksave()
	ptr := c.builder.CreateAlloca(c.types.ToLLVM(typ), "")
	args := []llvm.Value{
		coerce(c.builder, v.LLVMValue(), c.runtime.eface.llvm),
		c.builder.CreatePtrToInt(rtyp, c.target.IntPtrType(), ""),
		c.builder.CreatePtrToInt(ptr, c.target.IntPtrType(), ""),
	}
	c.builder.CreateCall(f, args, "")
	res := c.builder.CreateLoad(ptr, "")
	c.stackrestore(stackptr)
	return c.NewValue(res, typ)
}

// convertI2E converts a non-empty interface value to an empty interface.
func (v *LLVMValue) convertI2E() *LLVMValue {
	c := v.compiler
	f := c.runtime.convertI2E.LLVMValue()
	args := []llvm.Value{coerce(c.builder, v.LLVMValue(), c.runtime.iface.llvm)}
	typ := types.NewInterface(nil, nil)
	return c.NewValue(coerce(c.builder, c.builder.CreateCall(f, args, ""), c.llvmtypes.ToLLVM(typ)), typ)
}

// convertE2I converts an empty interface value to a non-empty interface.
func (v *LLVMValue) convertE2I(iface types.Type) (result, success *LLVMValue) {
	c := v.compiler
	f := c.runtime.convertE2I.LLVMValue()
	typ := c.builder.CreatePtrToInt(c.types.ToRuntime(iface), c.target.IntPtrType(), "")
	args := []llvm.Value{coerce(c.builder, v.LLVMValue(), c.runtime.eface.llvm), typ}
	res := c.builder.CreateCall(f, args, "")
	succ := c.builder.CreateExtractValue(res, 0, "")
	res = coerce(c.builder, c.builder.CreateExtractValue(res, 1, ""), c.types.ToLLVM(iface))
	return c.NewValue(res, iface), c.NewValue(succ, types.Typ[types.Bool])
}

// mustConvertE2I calls convertE2I, panicking if the assertion failed.
func (v *LLVMValue) mustConvertE2I(iface types.Type) *LLVMValue {
	c := v.compiler
	f := c.runtime.mustConvertE2I.LLVMValue()
	typ := c.builder.CreatePtrToInt(c.types.ToRuntime(iface), c.target.IntPtrType(), "")
	args := []llvm.Value{coerce(c.builder, v.LLVMValue(), c.runtime.eface.llvm), typ}
	res := coerce(c.builder, c.builder.CreateCall(f, args, ""), c.types.ToLLVM(iface))
	return c.NewValue(res, iface)
}
