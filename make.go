// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"fmt"
	"github.com/axw/gollvm/llvm"
	"go/ast"
)

func (c *compiler) VisitMake(expr *ast.CallExpr) Value {
	typ := c.typeinfo.Types[expr]
	switch utyp := typ.Underlying().(type) {
	case *types.Slice:
		var length, capacity Value
		switch len(expr.Args) {
		case 3:
			capacity = c.VisitExpr(expr.Args[2])
			fallthrough
		case 2:
			length = c.VisitExpr(expr.Args[1])
		}
		slice := c.makeSlice(utyp.Elem(), length, capacity)
		return c.NewValue(slice, typ)
	case *types.Chan:
		f := c.NamedFunction("runtime.makechan", "func(t uintptr, cap int) uintptr")
		dyntyp := c.types.ToRuntime(typ)
		dyntyp = c.builder.CreatePtrToInt(dyntyp, c.target.IntPtrType(), "")
		var cap_ llvm.Value
		if len(expr.Args) > 1 {
			cap_ = c.VisitExpr(expr.Args[1]).LLVMValue()
		} else {
			cap_ = llvm.ConstNull(c.types.inttype)
		}
		args := []llvm.Value{dyntyp, cap_}
		ptr := c.builder.CreateCall(f, args, "")
		return c.NewValue(ptr, typ)
	case *types.Map:
		return c.makeMapLiteral(typ, nil, nil)
	}
	panic(fmt.Sprintf("unhandled type: %s", typ))
}

// vim: set ft=go :
