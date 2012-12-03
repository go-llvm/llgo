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
	"./types"
	"go/token"
)

func (c *compiler) coerceString(v llvm.Value, typ llvm.Type) llvm.Value {
	result := llvm.Undef(typ)
	ptr := c.builder.CreateExtractValue(v, 0, "")
	len := c.builder.CreateExtractValue(v, 1, "")
	result = c.builder.CreateInsertValue(result, ptr, 0, "")
	result = c.builder.CreateInsertValue(result, len, 1, "")
	return result
}

func (c *compiler) concatenateStrings(lhs, rhs *LLVMValue) *LLVMValue {
	strcat := c.NamedFunction("runtime.strcat", "func f(a, b _string) _string")
	_string := strcat.Type().ElementType().ReturnType()
	lhsstr := c.coerceString(lhs.LLVMValue(), _string)
	rhsstr := c.coerceString(rhs.LLVMValue(), _string)
	args := []llvm.Value{lhsstr, rhsstr}
	result := c.builder.CreateCall(strcat, args, "")
	result = c.coerceString(result, c.types.ToLLVM(types.String))
	return c.NewLLVMValue(result, types.String)
}

func (c *compiler) compareStrings(lhs, rhs *LLVMValue, op token.Token) *LLVMValue {
	strcmp := c.NamedFunction("runtime.strcmp", "func f(a, b _string) int32")
	_string := strcmp.Type().ElementType().ParamTypes()[0]
	lhsstr := c.coerceString(lhs.LLVMValue(), _string)
	rhsstr := c.coerceString(rhs.LLVMValue(), _string)
	args := []llvm.Value{lhsstr, rhsstr}
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

func (c *compiler) stringNext(strval, index llvm.Value) (consumed, value llvm.Value) {
	strnext := c.NamedFunction("runtime.strnext", "func f(s _string, i int) (int, rune)")
	_string := strnext.Type().ElementType().ParamTypes()[0]
	strval = c.coerceString(strval, _string)
	args := []llvm.Value{strval, index}
	result := c.builder.CreateCall(strnext, args, "")
	consumed = c.builder.CreateExtractValue(result, 0, "")
	value = c.builder.CreateExtractValue(result, 1, "")
	return
}

func (v *LLVMValue) runeToString() *LLVMValue {
	c := v.compiler
	strrune := c.NamedFunction("runtime.strrune", "func f(n int64) _string")
	args := []llvm.Value{v.Convert(types.Int64).LLVMValue()}
	result := c.builder.CreateCall(strrune, args, "")
	result = c.coerceString(result, c.types.ToLLVM(types.String))
	return c.NewLLVMValue(result, types.String)
}

// vim: set ft=go:
