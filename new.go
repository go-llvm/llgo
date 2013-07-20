// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"go/ast"
)

func (c *compiler) VisitNew(expr *ast.CallExpr) Value {
	if len(expr.Args) > 1 {
		panic("Expecting only one argument to new")
	}
	ptrtyp := c.typeinfo.Types[expr].(*types.Pointer)
	typ := ptrtyp.Elem()
	llvmtyp := c.types.ToLLVM(typ)
	mem := c.createTypeMalloc(llvmtyp)
	return c.NewValue(mem, ptrtyp)
}

// vim: set ft=go :
