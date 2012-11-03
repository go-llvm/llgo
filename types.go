// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"go/ast"
)

// XXX the below function is a clone of the one from llgo/types.
// this is used to check whether a "function call" is in fact a
// type conversion.

// isType checks if an expression is a type.
func isType(x ast.Expr) bool {
	switch t := x.(type) {
	case *ast.Ident:
		return t.Obj != nil && t.Obj.Kind == ast.Typ
	case *ast.ParenExpr:
		return isType(t.X)
	case *ast.SelectorExpr:
		// qualified identifier
		if ident, ok := t.X.(*ast.Ident); ok {
			if obj := ident.Obj; obj != nil {
				if obj.Kind != ast.Pkg {
					return false
				}
				pkgscope := obj.Data.(*ast.Scope)
				obj := pkgscope.Lookup(t.Sel.Name)
				return obj != nil && obj.Kind == ast.Typ
			}
		}
		return false
	case *ast.StarExpr:
		return isType(t.X)
	case *ast.ArrayType,
		*ast.StructType,
		*ast.FuncType,
		*ast.InterfaceType,
		*ast.MapType,
		*ast.ChanType:
		return true
	}
	return false
}
