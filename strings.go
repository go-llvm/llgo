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
	"go/token"
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo/types"
)

func (c *compiler) concatenateStrings(lhs, rhs *LLVMValue) *LLVMValue {
	strcat := c.module.Module.NamedFunction("runtime.strcat")
	if strcat.IsNil() {
		string_type := c.types.ToLLVM(types.String)
		param_types := []llvm.Type{string_type, string_type}
		func_type := llvm.FunctionType(string_type, param_types, false)
		strcat = llvm.AddFunction(c.module.Module, "runtime.strcat", func_type)
	}
	args := []llvm.Value{lhs.LLVMValue(), rhs.LLVMValue()}
	result := c.builder.CreateCall(strcat, args, "")
	return c.NewLLVMValue(result, types.String)
}

func (c *compiler) compareStrings(lhs, rhs *LLVMValue, op token.Token) *LLVMValue {
    strcmp := c.module.Module.NamedFunction("runtime.strcmp")
    if strcmp.IsNil() {
        string_type := c.types.ToLLVM(types.String)
        param_types := []llvm.Type{string_type, string_type}
        func_type := llvm.FunctionType(llvm.Int32Type(), param_types, false)
        strcmp = llvm.AddFunction(c.module.Module, "runtime.strcmp", func_type)
    }
    args := []llvm.Value{lhs.LLVMValue(), rhs.LLVMValue()}
	result := c.builder.CreateCall(strcmp, args, "")
	zero := llvm.ConstNull(llvm.Int32Type())
	var pred llvm.IntPredicate
	switch op {
	case token.EQL:
		pred = llvm.IntEQ
	case token.LSS:
		pred = llvm.IntSLT
	case token.GTR:
		pred = llvm.IntSGT
	case token.LEQ:
		pred = llvm.IntSLE
	case token.GEQ:
		pred = llvm.IntSGE
	case token.NEQ:
		panic("NEQ is handled in LLVMValue.BinaryOp")
	default:
		panic("unreachable")
	}
	result = c.builder.CreateICmp(pred, result, zero, "")
    return c.NewLLVMValue(result, types.Bool)
}

// vim: set ft=go:
