// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.exp/go/types"
	"github.com/axw/gollvm/llvm"
)

func (c *compiler) visitRecover() *LLVMValue {
	eface := &types.Interface{}
	err := c.builder.CreateAlloca(c.types.ToLLVM(eface), "")
	f := c.NamedFunction("runtime.recover", "func f(*interface{})")
	c.builder.CreateCall(f, []llvm.Value{err}, "")
	return c.NewValue(c.builder.CreateLoad(err, ""), eface)
}

func (c *compiler) visitPanic(arg Value) {
	panic_ := c.NamedFunction("runtime.panic_", "func f(interface{})")
	args := []llvm.Value{arg.Convert(&types.Interface{}).LLVMValue()}
	if f := c.functions.top(); !f.unwindblock.IsNil() {
		c.builder.CreateInvoke(panic_, args, f.deferblock, f.unwindblock, "")
	} else {
		c.builder.CreateCall(panic_, args, "")
		c.builder.CreateUnreachable()
	}
}
