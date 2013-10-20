// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"github.com/axw/gollvm/llvm"
)

// makeClosure creates a closure from a function pointer and
// a set of bindings. The bindings are addresses of captured
// variables.
func (c *compiler) makeClosure(fn *LLVMValue, bindings []*LLVMValue) *LLVMValue {
	types := make([]llvm.Type, len(bindings))
	for i, binding := range bindings {
		types[i] = c.types.ToLLVM(binding.Type())
	}
	block := c.createTypeMalloc(llvm.StructType(types, false))
	for i, binding := range bindings {
		addressPtr := c.builder.CreateStructGEP(block, i, "")
		c.builder.CreateStore(binding.LLVMValue(), addressPtr)
	}
	block = c.builder.CreateBitCast(block, llvm.PointerType(llvm.Int8Type(), 0), "")
	// fn is a raw function pointer; ToLLVM yields {*fn, *uint8}.
	closure := llvm.Undef(c.types.ToLLVM(fn.Type()))
	closure = c.builder.CreateInsertValue(closure, fn.LLVMValue(), 0, "")
	closure = c.builder.CreateInsertValue(closure, block, 1, "")
	return c.NewValue(closure, fn.Type())
}
