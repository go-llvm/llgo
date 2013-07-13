// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

func (c *compiler) visitRecover() *LLVMValue {
	// Functions that call recover must not be inlined, or we
	// can't tell whether the recover call is valid.
	fn := c.functions.top()
	fnptr := c.builder.CreateExtractValue(fn.value, 0, "")
	fnptr.AddFunctionAttr(llvm.NoInlineAttribute)

	// We need to tell runtime.recover if it's being called from
	// an indirectly invoked deferred function or not.
	var indirect llvm.Value
	sig := fn.Type().(*types.Signature)
	if sig.Params().Len() == 0 {
		indirect = llvm.ConstInt(llvm.Int32Type(), 0, false)
	} else {
		indirect = llvm.ConstInt(llvm.Int32Type(), 1, false)
	}
	eface := &types.Interface{}
	err := c.builder.CreateAlloca(c.types.ToLLVM(eface), "")
	r := c.NamedFunction("runtime.recover", "func(int32, *interface{})")
	c.builder.CreateCall(r, []llvm.Value{indirect, err}, "")
	return c.NewValue(c.builder.CreateLoad(err, ""), eface)
}

func (c *compiler) visitPanic(arg Value) {
	panic_ := c.NamedFunction("runtime.panic_", "func(interface{})")
	args := []llvm.Value{arg.Convert(&types.Interface{}).LLVMValue()}
	if f := c.functions.top(); f != nil && !f.unwindblock.IsNil() {
		c.builder.CreateInvoke(panic_, args, f.deferblock, f.unwindblock, "")
	} else {
		c.builder.CreateCall(panic_, args, "")
		c.builder.CreateUnreachable()
	}
}
