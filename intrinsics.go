// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"github.com/axw/gollvm/llvm"
	"strconv"
)

func (c *compiler) defineRuntimeIntrinsics() {
	fn := c.module.NamedFunction("runtime.free")
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
	memset := c.NamedFunction("runtime.memset", "func f(dst unsafe.Pointer, fill byte, size uintptr)")
	switch n := size.Type().IntTypeWidth() - c.target.IntPtrType().IntTypeWidth(); {
	case n < 0:
		size = c.builder.CreateZExt(size, c.target.IntPtrType(), "")
	case n > 0:
		size = c.builder.CreateTrunc(size, c.target.IntPtrType(), "")
	}
	ptr = c.builder.CreatePtrToInt(ptr, c.target.IntPtrType(), "")
	fill := llvm.ConstNull(llvm.Int8Type())
	c.builder.CreateCall(memset, []llvm.Value{ptr, fill, size}, "")
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
	memcpy := c.NamedFunction(memcpyName, "func f(dst, src *int8, size uintptr, align int32, volatile bool)")

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
	memset := c.NamedFunction(memsetName, "func f(dst *int8, fill byte, size uintptr, align int32, volatile bool)")
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
