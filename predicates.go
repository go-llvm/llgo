// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements commonly used type predicates.

package llgo

import (
	"go/types"
)

func isNamed(typ types.Type) bool {
	if _, ok := typ.(*types.Basic); ok {
		return ok
	}
	_, ok := typ.(*types.NamedType)
	return ok
}

func isBoolean(typ types.Type) bool {
	t, ok := underlyingType(typ).(*types.Basic)
	return ok && t.Info&types.IsBoolean != 0
}

func isInteger(typ types.Type) bool {
	t, ok := underlyingType(typ).(*types.Basic)
	return ok && t.Info&types.IsInteger != 0
}

func isUnsigned(typ types.Type) bool {
	t, ok := underlyingType(typ).(*types.Basic)
	return ok && t.Info&types.IsUnsigned != 0
}

func isFloat(typ types.Type) bool {
	t, ok := underlyingType(typ).(*types.Basic)
	return ok && t.Info&types.IsFloat != 0
}

func isComplex(typ types.Type) bool {
	t, ok := underlyingType(typ).(*types.Basic)
	return ok && t.Info&types.IsComplex != 0
}

func isNumeric(typ types.Type) bool {
	t, ok := underlyingType(typ).(*types.Basic)
	return ok && t.Info&types.IsNumeric != 0
}

func isString(typ types.Type) bool {
	t, ok := underlyingType(typ).(*types.Basic)
	return ok && t.Info&types.IsString != 0
}

func isUntyped(typ types.Type) bool {
	t, ok := underlyingType(typ).(*types.Basic)
	return ok && t.Info&types.IsUntyped != 0
}

func isOrdered(typ types.Type) bool {
	t, ok := underlyingType(typ).(*types.Basic)
	return ok && t.Info&types.IsOrdered != 0
}

func isConstType(typ types.Type) bool {
	t, ok := underlyingType(typ).(*types.Basic)
	return ok && t.Info&types.IsConstType != 0
}

func isComparable(typ types.Type) bool {
	switch t := underlyingType(typ).(type) {
	case *types.Basic:
		return t.Kind != types.Invalid && t.Kind != types.UntypedNil
	case *types.Pointer, *types.Interface, *types.Chan:
		// assumes types are equal for pointers and channels
		return true
	case *types.Struct:
		for _, f := range t.Fields {
			if !isComparable(f.Type) {
				return false
			}
		}
		return true
	case *types.Array:
		return isComparable(t.Elt)
	}
	return false
}

func hasNil(typ types.Type) bool {
	switch underlyingType(typ).(type) {
	case *types.Slice, *types.Pointer, *types.Signature, *types.Interface, *types.Map, *types.Chan:
		return true
	}
	return false
}

// identical returns true if x and y are identical.
func isIdentical(x, y types.Type) bool {
	if x == y {
		return true
	}

	switch x := x.(type) {
	case *types.Basic:
		// Basic types are singletons except for the rune and byte
		// aliases, thus we cannot solely rely on the x == y check
		// above.
		if y, ok := y.(*types.Basic); ok {
			return x.Kind == y.Kind
		}

	case *types.Array:
		// Two array types are identical if they have identical element types
		// and the same array length.
		if y, ok := y.(*types.Array); ok {
			return x.Len == y.Len && isIdentical(x.Elt, y.Elt)
		}

	case *types.Slice:
		// Two slice types are identical if they have identical element types.
		if y, ok := y.(*types.Slice); ok {
			return isIdentical(x.Elt, y.Elt)
		}

	case *types.Struct:
		// Two struct types are identical if they have the same sequence of fields,
		// and if corresponding fields have the same names, and identical types,
		// and identical tags. Two anonymous fields are considered to have the same
		// name. Lower-case field names from different packages are always different.
		if y, ok := y.(*types.Struct); ok {
			// TODO(gri) handle structs from different packages
			if len(x.Fields) == len(y.Fields) {
				for i, f := range x.Fields {
					g := y.Fields[i]
					if f.Name != g.Name ||
						!isIdentical(f.Type, g.Type) ||
						f.Tag != g.Tag ||
						f.IsAnonymous != g.IsAnonymous {
						return false
					}
				}
				return true
			}
		}

	case *types.Pointer:
		// Two pointer types are identical if they have identical base types.
		if y, ok := y.(*types.Pointer); ok {
			return isIdentical(x.Base, y.Base)
		}

	case *types.Signature:
		// Two function types are identical if they have the same number of parameters
		// and result values, corresponding parameter and result types are identical,
		// and either both functions are variadic or neither is. Parameter and result
		// names are not required to match.
		if y, ok := y.(*types.Signature); ok {
			return identicalTypes(x.Params, y.Params) &&
				identicalTypes(x.Results, y.Results) &&
				x.IsVariadic == y.IsVariadic
		}

	case *types.Interface:
		// Two interface types are identical if they have the same set of methods with
		// the same names and identical function types. Lower-case method names from
		// different packages are always different. The order of the methods is irrelevant.
		if y, ok := y.(*types.Interface); ok {
			return identicalMethods(x.Methods, y.Methods) // methods are sorted
		}

	case *types.Map:
		// Two map types are identical if they have identical key and value types.
		if y, ok := y.(*types.Map); ok {
			return isIdentical(x.Key, y.Key) && isIdentical(x.Elt, y.Elt)
		}

	case *types.Chan:
		// Two channel types are identical if they have identical value types
		// and the same direction.
		if y, ok := y.(*types.Chan); ok {
			return x.Dir == y.Dir && isIdentical(x.Elt, y.Elt)
		}

	case *types.NamedType:
		// Two named types are identical if their type names originate
		// in the same type declaration.
		if y, ok := y.(*types.NamedType); ok {
			return x.Obj == y.Obj
		}
	}

	return false
}

// identicalTypes returns true if both lists a and b have the
// same length and corresponding objects have identical types.
func identicalTypes(a, b []*types.Var) bool {
	if len(a) != len(b) {
		return false
	}
	for i, x := range a {
		y := b[i]
		if !isIdentical(x.Type, y.Type) {
			return false
		}
	}
	return true
}

// identicalMethods returns true if both lists a and b have the
// same length and corresponding methods have identical types.
// TODO(gri) make this more efficient
func identicalMethods(a, b []*types.Method) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]*types.Method)
	for _, x := range a {
		m[x.Name] = x
	}
	for _, y := range b {
		if x := m[y.Name]; x == nil || !isIdentical(x.Type, y.Type) {
			return false
		}
	}
	return true
}
