/*
Copyright (c) 2012 Andrew Wilkins <axwalk@gmail.com>

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
	"go/ast"
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo/types"
)

// makeSlice allocates a new slice, storing in it the provided elements.
func (c *compiler) makeSlice(v []llvm.Value, elttyp types.Type) llvm.Value {
	n := llvm.ConstInt(llvm.Int32Type(), uint64(len(v)), false)
	llvmelttyp := c.types.ToLLVM(elttyp)
	mem := c.builder.CreateArrayMalloc(llvmelttyp, n, "")
	for i, value := range v {
		indices := []llvm.Value{llvm.ConstInt(llvm.Int32Type(), uint64(i), false)}
		ep := c.builder.CreateGEP(mem, indices, "")
		c.builder.CreateStore(value, ep)
	}
	slicetyp := types.Slice{Elt: elttyp}
	struct_ := llvm.Undef(c.types.ToLLVM(&slicetyp))
	struct_ = c.builder.CreateInsertValue(struct_, mem, 0, "")
	struct_ = c.builder.CreateInsertValue(struct_, n, 1, "")
	struct_ = c.builder.CreateInsertValue(struct_, n, 2, "")
	return struct_
}

// coerceSlice takes a slice of one element type and coerces it to a
// slice of another.
func (c *compiler) coerceSlice(src llvm.Value, dsttyp llvm.Type) llvm.Value {
	dst := llvm.Undef(dsttyp)
	srcmem := c.builder.CreateExtractValue(src, 0, "")
	srclen := c.builder.CreateExtractValue(src, 1, "")
	srccap := c.builder.CreateExtractValue(src, 2, "")
	dstmemtyp := dsttyp.StructElementTypes()[0]
	dstmem := c.builder.CreateBitCast(srcmem, dstmemtyp, "")
	dst = c.builder.CreateInsertValue(dst, dstmem, 0, "")
	dst = c.builder.CreateInsertValue(dst, srclen, 1, "")
	dst = c.builder.CreateInsertValue(dst, srccap, 2, "")
	return dst
}

func (c *compiler) VisitAppend(expr *ast.CallExpr) Value {
	// TODO handle ellpisis arg
	s := c.VisitExpr(expr.Args[0])
	elem := c.VisitExpr(expr.Args[1])

	appendName := "runtime.sliceappend"
	appendFun := c.module.NamedFunction(appendName)
	uintptrTyp := c.target.IntPtrType()
	var i8slice llvm.Type
	if appendFun.IsNil() {
		i8slice = c.types.ToLLVM(&types.Slice{Elt: types.Int8})
		args := []llvm.Type{uintptrTyp, i8slice, i8slice}
		appendFunTyp := llvm.FunctionType(i8slice, args, false)
		appendFun = llvm.AddFunction(c.module.Module, appendName, appendFunTyp)
	} else {
		i8slice = appendFun.Type().ReturnType()
	}
	i8ptr := i8slice.StructElementTypes()[0]

	// Coerce first argument into an []int8.
	a_ := s.LLVMValue()
	sliceTyp := a_.Type()
	a := c.coerceSlice(a_, i8slice)

	// Construct a fresh []int8 for the temporary slice.
	b_ := elem.LLVMValue()
	one := llvm.ConstInt(llvm.Int32Type(), 1, false)
	mem := c.builder.CreateAlloca(elem.LLVMValue().Type(), "")
	c.builder.CreateStore(b_, mem)
	b := llvm.Undef(i8slice)
	b = c.builder.CreateInsertValue(b, c.builder.CreateBitCast(mem, i8ptr, ""), 0, "")
	b = c.builder.CreateInsertValue(b, one, 1, "")
	b = c.builder.CreateInsertValue(b, one, 2, "")

	// Call runtime function, then coerce the result.
	runtimeTyp := c.types.ToRuntime(s.Type())
	runtimeTyp = c.builder.CreatePtrToInt(runtimeTyp, uintptrTyp, "")
	args := []llvm.Value{runtimeTyp, a, b}
	result := c.builder.CreateCall(appendFun, args, "")
	return c.NewLLVMValue(c.coerceSlice(result, sliceTyp), s.Type())
}

// vim: set ft=go :
