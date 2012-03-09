/*
Copyright (c) 2011, 2012 Andrew Wilkins <axwalk@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

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
