// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"./types"
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

func (c *compiler) exportBuiltinRuntimeTypes() {
	types := []types.Type{
		types.Uint,
		types.Uint8,
		types.Uint16,
		types.Uint32,
		types.Uint64,
		types.Int,
		types.Int8,
		types.Int16,
		types.Int32,
		types.Int64,
		types.Float32,
		types.Float64,
		types.Complex64,
		types.Complex128,
		types.Byte,
		types.Bool,
		types.Uintptr,
		types.Rune,
		types.UnsafePointer,
		types.String,
		types.Error,
	}
	for _, typ := range types {
		c.types.ToRuntime(typ)
	}
}
