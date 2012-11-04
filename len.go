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
	"github.com/axw/llgo/types"
	"go/ast"
	"go/token"
	"strconv"
	"unsafe"
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
	if name, ok := typ.(*types.Name); ok {
		typ = name.Underlying
	}

	switch typ := typ.(type) {
	case *types.Pointer:
		// XXX Converting to a string to be converted back to an int is
		// silly. The values need an overhaul? Perhaps have types based
		// on fundamental types, with the additional methods to make
		// them llgo.Value's.
		if a, isarray := typ.Base.(*types.Array); isarray {
			return c.NewConstValue(token.INT,
				strconv.FormatUint(a.Len, 10))
		}
		v := strconv.FormatUint(uint64(unsafe.Sizeof(uintptr(0))), 10)
		return c.NewConstValue(token.INT, v)

	case *types.Slice:
		//ptr := value.(*LLVMValue).pointer
		//len_field := c.builder.CreateStructGEP(ptr.LLVMValue(), 1, "")
		sliceval := value.LLVMValue()
		lenval := c.builder.CreateExtractValue(sliceval, 1, "") //c.builder.CreateLoad(len_field, "")
		return c.NewLLVMValue(lenval, types.Int32).Convert(types.Int)

	case *types.Map:
		mapval := value.LLVMValue()
		lenval := c.builder.CreateExtractValue(mapval, 0, "") //c.builder.CreateLoad(len_field, "")
		return c.NewLLVMValue(lenval, types.Int32).Convert(types.Int)

	case *types.Array:
		v := strconv.FormatUint(typ.Len, 10)
		return c.NewConstValue(token.INT, v)

	case *types.Basic:
		if typ == types.String.Underlying {
			switch value := value.(type) {
			case *LLVMValue:
				ptr := value.pointer
				len_field := c.builder.CreateStructGEP(ptr.LLVMValue(), 1, "")
				len_value := c.builder.CreateLoad(len_field, "")
				return c.NewLLVMValue(len_value, types.Int32).Convert(types.Int)
			case ConstValue:
				s := value.Val.(string)
				n := uint64(len(s))
				return c.NewConstValue(token.INT, strconv.FormatUint(n, 10))
			}
		}
	}
	panic(fmt.Sprint("Unhandled value type: ", value.Type()))
}

// vim: set ft=go :
