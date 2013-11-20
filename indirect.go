// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// indirectFunction creates an indirect function from a
// given function and arguments, suitable for use with
// "defer" and "go".
func (c *compiler) indirectFunction(fn *LLVMValue, args []*LLVMValue) *LLVMValue {
	nilarytyp := types.NewSignature(nil, nil, nil, nil, false)
	if len(args) == 0 {
		val := fn.LLVMValue()
		ptr := c.builder.CreateExtractValue(val, 0, "")
		ctx := c.builder.CreateExtractValue(val, 1, "")
		fnval := llvm.Undef(c.types.ToLLVM(nilarytyp))
		ptr = c.builder.CreateBitCast(ptr, fnval.Type().StructElementTypes()[0], "")
		ctx = c.builder.CreateBitCast(ctx, fnval.Type().StructElementTypes()[1], "")
		fnval = c.builder.CreateInsertValue(fnval, ptr, 0, "")
		fnval = c.builder.CreateInsertValue(fnval, ctx, 1, "")
		fn = c.NewValue(fnval, nilarytyp)
		return fn
	}

	// TODO check if function pointer is global. I suppose
	// the same can be done with the context ptr...
	fnval := fn.LLVMValue()
	fnptr := c.builder.CreateExtractValue(fnval, 0, "")
	ctx := c.builder.CreateExtractValue(fnval, 1, "")
	nctx := 1 // fnptr
	if !ctx.IsNull() {
		nctx++ // fnctx
	}

	i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
	llvmargs := make([]llvm.Value, len(args)+nctx)
	llvmargtypes := make([]llvm.Type, len(args)+nctx)
	for i, arg := range args {
		llvmargs[i+nctx] = arg.LLVMValue()
		llvmargtypes[i+nctx] = llvmargs[i+nctx].Type()
	}
	llvmargtypes[0] = fnptr.Type()
	llvmargs[0] = fnptr
	if nctx > 1 {
		llvmargtypes[1] = ctx.Type()
		llvmargs[1] = ctx
	}

	// TODO(axw) investigate an option for go statements
	// to allocate argument structure on the stack in the
	// initiator, and block until the spawned goroutine
	// has loaded the arguments from it.
	structtyp := llvm.StructType(llvmargtypes, false)
	argstruct := c.createTypeMalloc(structtyp)
	for i, llvmarg := range llvmargs {
		argptr := c.builder.CreateGEP(argstruct, []llvm.Value{
			llvm.ConstInt(llvm.Int32Type(), 0, false),
			llvm.ConstInt(llvm.Int32Type(), uint64(i), false)}, "")
		c.builder.CreateStore(llvmarg, argptr)
	}

	// Create a function that will take a pointer to a structure of the type
	// defined above, or no parameters if there are none to pass.
	fntype := llvm.FunctionType(llvm.VoidType(), []llvm.Type{argstruct.Type()}, false)
	indirectfn := llvm.AddFunction(c.module.Module, "", fntype)
	i8argstruct := c.builder.CreateBitCast(argstruct, i8ptr, "")
	currblock := c.builder.GetInsertBlock()
	c.builder.SetInsertPointAtEnd(llvm.AddBasicBlock(indirectfn, "entry"))
	argstruct = indirectfn.Param(0)
	newargs := make([]*LLVMValue, len(args))
	for i := range llvmargs[nctx:] {
		argptr := c.builder.CreateGEP(argstruct, []llvm.Value{
			llvm.ConstInt(llvm.Int32Type(), 0, false),
			llvm.ConstInt(llvm.Int32Type(), uint64(i+nctx), false)}, "")
		newargs[i] = c.NewValue(c.builder.CreateLoad(argptr, ""), args[i].Type())
	}

	// Extract the function pointer.
	// TODO if function is a global, elide.
	fnval = llvm.Undef(fnval.Type())
	fnptrptr := c.builder.CreateGEP(argstruct, []llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), 0, false),
		llvm.ConstInt(llvm.Int32Type(), 0, false)}, "")
	fnptr = c.builder.CreateLoad(fnptrptr, "")
	fnval = c.builder.CreateInsertValue(fnval, fnptr, 0, "")
	if nctx > 1 {
		ctxptr := c.builder.CreateGEP(argstruct, []llvm.Value{
			llvm.ConstInt(llvm.Int32Type(), 0, false),
			llvm.ConstInt(llvm.Int32Type(), 1, false)}, "")
		ctx = c.builder.CreateLoad(ctxptr, "")
		fnval = c.builder.CreateInsertValue(fnval, ctx, 1, "")
		fn = c.NewValue(fnval, fn.Type())
	}
	c.createCall(fn, newargs)

	// Indirect function calls' return values are always ignored.
	c.builder.CreateRetVoid()
	c.builder.SetInsertPointAtEnd(currblock)

	fnval = llvm.Undef(c.types.ToLLVM(nilarytyp))
	indirectfn = c.builder.CreateBitCast(indirectfn, fnval.Type().StructElementTypes()[0], "")
	fnval = c.builder.CreateInsertValue(fnval, indirectfn, 0, "")
	fnval = c.builder.CreateInsertValue(fnval, i8argstruct, 1, "")
	fn = c.NewValue(fnval, nilarytyp)
	return fn
}
