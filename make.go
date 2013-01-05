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
	"fmt"
	"github.com/axw/gollvm/llvm"
	"go/ast"
	"go/types"
)

func (c *compiler) VisitMake(expr *ast.CallExpr) Value {
	typ := c.types.expr[expr].Type
	switch utyp := underlyingType(typ).(type) {
	case *types.Slice:
		var length, capacity Value
		switch len(expr.Args) {
		case 3:
			capacity = c.VisitExpr(expr.Args[2])
			fallthrough
		case 2:
			length = c.VisitExpr(expr.Args[1])
		}
		slice := c.makeSlice(utyp.Elt, length, capacity)
		return c.NewValue(slice, typ)
	case *types.Chan:
		f := c.NamedFunction("runtime.makechan", "func f(t uintptr, cap int) uintptr")
		dyntyp := c.types.ToRuntime(typ)
		dyntyp = c.builder.CreatePtrToInt(dyntyp, c.target.IntPtrType(), "")
		var cap_ llvm.Value
		if len(expr.Args) > 1 {
			cap_ = c.VisitExpr(expr.Args[1]).LLVMValue()
		}
		args := []llvm.Value{dyntyp, cap_}
		ptr := c.builder.CreateCall(f, args, "")
		return c.NewValue(ptr, typ)
	case *types.Map:
		f := c.NamedFunction("runtime.makemap", "func f(t uintptr) uintptr")
		dyntyp := c.types.ToRuntime(typ)
		dyntyp = c.builder.CreatePtrToInt(dyntyp, c.target.IntPtrType(), "")
		mapval := c.builder.CreateCall(f, []llvm.Value{dyntyp}, "")
		return c.NewValue(mapval, typ)
	}
	panic(fmt.Sprintf("unhandled type: %s", typ))
}

// vim: set ft=go :
