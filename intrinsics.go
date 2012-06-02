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
	"strconv"
)

func (c *compiler) defineRuntimeIntrinsics() {
	fn := c.module.NamedFunction("runtime.malloc")
	if !fn.IsNil() {
		c.defineMallocFunction(fn)
	}

	fn = c.module.NamedFunction("runtime.memcpy")
	if !fn.IsNil() {
		c.defineMemcpyFunction(fn)
	}
}

func (c *compiler) defineMallocFunction(fn llvm.Value) {
	entry := llvm.AddBasicBlock(fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)
	size := fn.FirstParam()
	ptr := c.builder.CreateArrayMalloc(llvm.Int8Type(), size, "")
	// XXX memset to zero, or leave that to Go runtime code?
	fn_type := fn.Type().ElementType()
	result := c.builder.CreatePtrToInt(ptr, fn_type.ReturnType(), "")
	c.builder.CreateRet(result)
}

func (c *compiler) defineMemcpyFunction(fn llvm.Value) {
	entry := llvm.AddBasicBlock(fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)
	dst, src, size := fn.Param(0), fn.Param(1), fn.Param(2)

	pint8 := llvm.PointerType(llvm.Int8Type(), 0)
	dst = c.builder.CreateIntToPtr(dst, pint8, "")
	src = c.builder.CreateIntToPtr(src, pint8, "")

	sizeType := size.Type()
	sizeBits := sizeType.IntTypeWidth()
	memcpyName := "llvm.memcpy.p0i8.p0i8.i" + strconv.Itoa(sizeBits)
	memcpy := c.module.NamedFunction(memcpyName)
	if memcpy.IsNil() {
		paramtypes := []llvm.Type{
			pint8, pint8, size.Type(), llvm.Int32Type(), llvm.Int1Type()}
		memcpyType := llvm.FunctionType(llvm.VoidType(), paramtypes, false)
		memcpy = llvm.AddFunction(c.module.Module, memcpyName, memcpyType)
	}

	args := []llvm.Value{
		dst, src, size,
		llvm.ConstInt(llvm.Int32Type(), 1, false), // single byte alignment
		llvm.ConstInt(llvm.Int1Type(), 0, false),  // not volatile
	}
	c.builder.CreateCall(memcpy, args, "")
	c.builder.CreateRetVoid()
}

// vim: set ft=go:
