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

	fn = c.module.NamedFunction("runtime.free")
	if !fn.IsNil() {
		c.defineFreeFunction(fn)
	}

	fn = c.module.NamedFunction("runtime.memcpy")
	if !fn.IsNil() {
		c.defineMemcpyFunction(fn, "memcpy")
	}

	fn = c.module.NamedFunction("runtime.memmove")
	if !fn.IsNil() {
		c.defineMemcpyFunction(fn, "memmove")
	}

	fn = c.module.NamedFunction("runtime.memset")
	if !fn.IsNil() {
		c.defineMemsetFunction(fn)
	}
}

func (c *compiler) memsetZero(ptr llvm.Value, size llvm.Value) {
	memset := c.namedFunction("runtime.memset", "func f(dst unsafe.Pointer, fill byte, size int)")
	ptr = c.builder.CreatePtrToInt(ptr, c.target.IntPtrType(), "")
	fill := llvm.ConstNull(llvm.Int8Type())
	c.builder.CreateCall(memset, []llvm.Value{ptr, fill, size}, "")
}

func (c *compiler) defineMallocFunction(fn llvm.Value) {
	entry := llvm.AddBasicBlock(fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)
	size := fn.FirstParam()
	ptr := c.builder.CreateArrayMalloc(llvm.Int8Type(), size, "")
	c.memsetZero(ptr, size)
	fn_type := fn.Type().ElementType()
	result := c.builder.CreatePtrToInt(ptr, fn_type.ReturnType(), "")
	c.builder.CreateRet(result)
}

func (c *compiler) defineFreeFunction(fn llvm.Value) {
	entry := llvm.AddBasicBlock(fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)
	ptr := fn.FirstParam()
	ptrtyp := llvm.PointerType(llvm.Int8Type(), 0)
	c.builder.CreateFree(c.builder.CreateIntToPtr(ptr, ptrtyp, ""))
	c.builder.CreateRetVoid()
}

func (c *compiler) defineMemcpyFunction(fn llvm.Value, name string) {
	entry := llvm.AddBasicBlock(fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)
	dst, src, size := fn.Param(0), fn.Param(1), fn.Param(2)
	sizeType := size.Type()
	sizeBits := sizeType.IntTypeWidth()

	memcpyName := "llvm." + name + ".p0i8.p0i8.i" + strconv.Itoa(sizeBits)
	memcpy := c.namedFunction(memcpyName, "func f(dst, src *int8, size int, align int32, volatile bool)")

	pint8 := memcpy.Type().ElementType().ParamTypes()[0]
	dst = c.builder.CreateIntToPtr(dst, pint8, "")
	src = c.builder.CreateIntToPtr(src, pint8, "")
	args := []llvm.Value{
		dst, src, size,
		llvm.ConstInt(llvm.Int32Type(), 1, false), // single byte alignment
		llvm.ConstInt(llvm.Int1Type(), 0, false),  // not volatile
	}
	c.builder.CreateCall(memcpy, args, "")
	c.builder.CreateRetVoid()
}

func (c *compiler) defineMemsetFunction(fn llvm.Value) {
	entry := llvm.AddBasicBlock(fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)
	dst, fill, size := fn.Param(0), fn.Param(1), fn.Param(2)
	sizeType := size.Type()
	sizeBits := sizeType.IntTypeWidth()
	memsetName := "llvm.memset.p0i8.i" + strconv.Itoa(sizeBits)
	memset := c.namedFunction(memsetName, "func f(dst *int8, fill byte, size int, align int32, volatile bool)")
	pint8 := memset.Type().ElementType().ParamTypes()[0]
	dst = c.builder.CreateIntToPtr(dst, pint8, "")
	args := []llvm.Value{
		dst, fill, size,
		llvm.ConstInt(llvm.Int32Type(), 1, false), // single byte alignment
		llvm.ConstInt(llvm.Int1Type(), 0, false),  // not volatile
	}
	c.builder.CreateCall(memset, args, "")
	c.builder.CreateRetVoid()
}
