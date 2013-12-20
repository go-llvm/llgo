// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"fmt"
)

func deref(t types.Type) types.Type {
	return t.Underlying().(*types.Pointer).Elem()
}

func (c *compiler) exportRuntimeTypes(builtin bool) {
	if builtin {
		kinds := [...]types.BasicKind{
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
			types.Bool,
			types.Uintptr,
			types.UnsafePointer,
			types.String,
		}
		for _, kind := range kinds {
			c.exportedtypes = append(c.exportedtypes, types.Typ[kind])
		}
		error_ := types.Universe.Lookup("error").Type()
		c.exportedtypes = append(c.exportedtypes, error_)
	}
	for _, typ := range c.exportedtypes {
		c.types.ToRuntime(typ)
		c.types.ToRuntime(types.NewPointer(typ))
	}
}

// tupleType returns a struct type with anonymous
// fields with the specified types.
func tupleType(fieldTypes ...types.Type) types.Type {
	vars := make([]*types.Var, len(fieldTypes))
	for i, t := range fieldTypes {
		vars[i] = types.NewParam(0, nil, fmt.Sprintf("f%d", i), t)
	}
	return types.NewStruct(vars, nil)
}
