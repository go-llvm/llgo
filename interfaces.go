// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// interfaceMethod returns a function pointer for the specified
// interface and method pair.
func (c *compiler) interfaceMethod(iface *LLVMValue, method *types.Func) *LLVMValue {
	lliface := iface.LLVMValue()
	llvalue := c.builder.CreateExtractValue(lliface, 1, "")
	// TODO represent iface properly, as {*itab, value},
	// and extract interface pointer here.
	//llitab := ll

	// Strip receiver.
	sig := method.Type().(*types.Signature)
	sig = types.NewSignature(nil, nil, sig.Params(), sig.Results(), sig.IsVariadic())

	llfn := llvm.ConstNull(c.types.ToLLVM(sig))
	llfn = c.builder.CreateInsertValue(llfn, llvalue, 1, "")
	return c.NewValue(llfn, sig)
}

// compareInterfaces emits code to compare two interfaces for
// equality.
func (c *compiler) compareInterfaces(a_, b_ *LLVMValue) *LLVMValue {
	a, b := a_.LLVMValue(), b_.LLVMValue()
	// TODO I2I, I2E, etc. when we have interfaces implemented right.
	f := c.runtime.compareI2I.LLVMValue()
	return c.NewValue(c.builder.CreateCall(f, []llvm.Value{a, b}, ""), types.Typ[types.Bool])
}
