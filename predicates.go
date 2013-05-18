// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements commonly used type predicates.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
)

func isNamed(typ types.Type) bool {
	if _, ok := typ.(*types.Basic); ok {
		return ok
	}
	_, ok := typ.(*types.Named)
	return ok
}

func isBoolean(typ types.Type) bool {
	t, ok := typ.Underlying().(*types.Basic)
	return ok && t.Info()&types.IsBoolean != 0
}

func isInteger(typ types.Type) bool {
	t, ok := typ.Underlying().(*types.Basic)
	return ok && t.Info()&types.IsInteger != 0
}

func isUnsigned(typ types.Type) bool {
	t, ok := typ.Underlying().(*types.Basic)
	return ok && t.Info()&types.IsUnsigned != 0
}

func isFloat(typ types.Type) bool {
	t, ok := typ.Underlying().(*types.Basic)
	return ok && t.Info()&types.IsFloat != 0
}

func isComplex(typ types.Type) bool {
	t, ok := typ.Underlying().(*types.Basic)
	return ok && t.Info()&types.IsComplex != 0
}

func isNumeric(typ types.Type) bool {
	t, ok := typ.Underlying().(*types.Basic)
	return ok && t.Info()&types.IsNumeric != 0
}

func isString(typ types.Type) bool {
	t, ok := typ.Underlying().(*types.Basic)
	return ok && t.Info()&types.IsString != 0
}

func isUntyped(typ types.Type) bool {
	t, ok := typ.Underlying().(*types.Basic)
	return ok && t.Info()&types.IsUntyped != 0
}

func isOrdered(typ types.Type) bool {
	t, ok := typ.Underlying().(*types.Basic)
	return ok && t.Info()&types.IsOrdered != 0
}

func isConstType(typ types.Type) bool {
	t, ok := typ.Underlying().(*types.Basic)
	return ok && t.Info()&types.IsConstType != 0
}

// defaultType returns the default "typed" type for an "untyped" type;
// it returns the incoming type for all other types. If there is no
// corresponding untyped type, the result is Typ[Invalid].
//
func defaultType(typ types.Type) types.Type {
	if t, ok := typ.(*types.Basic); ok {
		k := types.Invalid
		switch t.Kind() {
		// case UntypedNil:
		//      There is no default type for nil. For a good error message,
		//      catch this case before calling this function.
		case types.UntypedBool:
			k = types.Bool
		case types.UntypedInt:
			k = types.Int
		case types.UntypedRune:
			k = types.Rune
		case types.UntypedFloat:
			k = types.Float64
		case types.UntypedComplex:
			k = types.Complex128
		case types.UntypedString:
			k = types.String
		}
		typ = types.Typ[k]
	}
	return typ
}
