// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"github.com/axw/gollvm/llvm"
	"go/types"
)

func (c *compiler) visitRecover() *LLVMValue {
	// TODO
	emptyInterface := &types.Interface{}
	errval := llvm.ConstNull(c.types.ToLLVM(emptyInterface))
	return c.NewValue(errval, emptyInterface)
}

func (c *compiler) visitPanic(arg Value) {
	f := c.NamedFunction("runtime.panic_", "func f(interface{})")
	arg = arg.Convert(&types.Interface{})
	c.builder.CreateCall(f, []llvm.Value{arg.LLVMValue()}, "")
	c.builder.CreateUnreachable()
}
