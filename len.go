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
	"fmt"
	"github.com/axw/gollvm/llvm"
	"go/ast"
	"go/types"
)

func (c *compiler) VisitCap(expr *ast.CallExpr) Value {
	// TODO implement me
	return c.VisitLen(expr)
}

func (c *compiler) VisitLen(expr *ast.CallExpr) Value {
	if len(expr.Args) > 1 {
		panic("Expecting only one argument to len")
	}

	value := c.VisitExpr(expr.Args[0])
	typ := value.Type()
	if name, ok := underlyingType(typ).(*types.NamedType); ok {
		typ = name.Underlying
	}

	var lenvalue llvm.Value
	switch typ := underlyingType(typ).(type) {
	case *types.Pointer:
		atyp := underlyingType(typ.Base).(*types.Array)
		lenvalue = llvm.ConstInt(c.llvmtypes.inttype, uint64(atyp.Len), false)

	case *types.Slice:
		sliceval := value.LLVMValue()
		lenvalue = c.builder.CreateExtractValue(sliceval, 1, "")

	case *types.Map:
		mapval := value.LLVMValue()
		f := c.NamedFunction("runtime.maplen", "func f(m uintptr) int")
		lenvalue = c.builder.CreateCall(f, []llvm.Value{mapval}, "")

	case *types.Array:
		lenvalue = llvm.ConstInt(c.llvmtypes.inttype, uint64(typ.Len), false)

	case *types.Basic:
		if isString(typ) {
			value := value.(*LLVMValue)
			ptr := value.pointer
			lenptr := c.builder.CreateStructGEP(ptr.LLVMValue(), 1, "")
			lenvalue = c.builder.CreateLoad(lenptr, "")
		}
	}
	if !lenvalue.IsNil() {
		return c.NewValue(lenvalue, types.Typ[types.Int])
	}
	panic(fmt.Sprint("Unhandled value type: ", value.Type()))
}

// vim: set ft=go :
