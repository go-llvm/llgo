// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"github.com/axw/gollvm/llvm"
)

func getnewgoroutine(module llvm.Module) llvm.Value {
	fn := module.NamedFunction("llgo_newgoroutine")
	if fn.IsNil() {
		i8Ptr := llvm.PointerType(llvm.Int8Type(), 0)
		VoidFnPtr := llvm.PointerType(llvm.FunctionType(
			llvm.VoidType(), []llvm.Type{i8Ptr}, false), 0)
		i32 := llvm.Int32Type()
		fn_type := llvm.FunctionType(
			llvm.VoidType(), []llvm.Type{VoidFnPtr, i8Ptr, i32}, true)
		fn = llvm.AddFunction(module, "llgo_newgoroutine", fn_type)
		fn.SetFunctionCallConv(llvm.CCallConv)
	}
	return fn
}

// vim: set ft=go :
