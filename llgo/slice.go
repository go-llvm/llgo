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
    //"go/ast"
    "github.com/axw/gollvm/llvm"
    "github.com/axw/llgo/types"
)

func (c *compiler) makeSlice(v []llvm.Value, typ *types.Slice) llvm.Value {
    n := llvm.ConstInt(llvm.Int32Type(), uint64(len(v)), false)
    mem := c.builder.CreateArrayMalloc(typ.Elt.LLVMType(), n, "")
    for i, value := range v {
        indices := []llvm.Value{
            llvm.ConstInt(llvm.Int32Type(), uint64(i), false)}
        ep := c.builder.CreateGEP(mem, indices, "")
        c.builder.CreateStore(value, ep)
    }
    struct_ := c.builder.CreateAlloca(typ.LLVMType(), "")
    c.builder.CreateStore(mem, c.builder.CreateStructGEP(struct_, 0, ""))
    c.builder.CreateStore(n, c.builder.CreateStructGEP(struct_, 1, ""))
    c.builder.CreateStore(n, c.builder.CreateStructGEP(struct_, 2, ""))
    return c.builder.CreateLoad(struct_, "")
}

// vim: set ft=go :

