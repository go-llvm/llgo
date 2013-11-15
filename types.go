// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"bytes"
	"code.google.com/p/go.tools/go/types"
	"fmt"
	"go/ast"
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
		vars[i] = types.NewParam(0, nil, "", t)
	}
	return types.NewStruct(vars, nil)
}

type TypeStringer struct{}

func (ts *TypeStringer) TypeKey(typ types.Type) string {
	var buf bytes.Buffer
	ts.writeType(&buf, typ, true)
	return buf.String()
}

// typeString returns a string representation for typ.
// below code based on go/types' typeString and friends.
func (ts *TypeStringer) TypeString(typ types.Type) string {
	var buf bytes.Buffer
	ts.writeType(&buf, typ, false)
	return buf.String()
}

func (ts *TypeStringer) writeParams(buf *bytes.Buffer, params *types.Tuple, isVariadic, unique bool) {
	buf.WriteByte('(')
	for i := 0; i < int(params.Len()); i++ {
		par := params.At(i)
		partyp := par.Type()
		if i > 0 {
			buf.WriteString(", ")
		}
		if isVariadic && i == int(params.Len()-1) {
			buf.WriteString("...")
			partyp = partyp.(*types.Slice).Elem()
		}
		ts.writeType(buf, partyp, unique)
	}
	buf.WriteByte(')')
}

func (ts *TypeStringer) writeSignature(buf *bytes.Buffer, sig *types.Signature, unique bool) {
	if recv := sig.Recv(); recv != nil {
		ts.writeType(buf, recv.Type(), unique)
		buf.WriteByte(' ')
	}

	ts.writeParams(buf, sig.Params(), sig.IsVariadic(), unique)
	if sig.Results().Len() == 0 {
		// no result
		return
	}

	buf.WriteByte(' ')
	if sig.Results().Len() == 1 {
		// single unnamed result
		ts.writeType(buf, sig.Results().At(0).Type(), unique)
		return
	}

	// multiple or named result(s)
	ts.writeParams(buf, sig.Results(), false, unique)
}

func (ts *TypeStringer) writeType(buf *bytes.Buffer, typ types.Type, unique bool) {
	switch t := typ.(type) {
	case nil:
		buf.WriteString("<nil>")

	case *types.Basic:
		// De-alias.
		t = types.Typ[t.Kind()]
		buf.WriteString(t.Name())

	case *types.Array:
		fmt.Fprintf(buf, "[%d]", t.Len())
		ts.writeType(buf, t.Elem(), unique)

	case *types.Slice:
		buf.WriteString("[]")
		ts.writeType(buf, t.Elem(), unique)

	case *types.Struct:
		buf.WriteString("struct{")
		for i := 0; i < t.NumFields(); i++ {
			f := t.Field(i)
			if i > 0 {
				buf.WriteString("; ")
			}
			if !f.Anonymous() {
				buf.WriteString(f.Name())
				buf.WriteByte(' ')
			}
			ts.writeType(buf, f.Type(), unique)
			if tag := t.Tag(i); tag != "" {
				fmt.Fprintf(buf, " %q", tag)
			}
		}
		buf.WriteByte('}')

	case *types.Pointer:
		buf.WriteByte('*')
		ts.writeType(buf, t.Elem(), unique)

	case *types.Tuple:
		ts.writeParams(buf, t, false, unique)

	case *types.Signature:
		buf.WriteString("func")
		ts.writeSignature(buf, t, unique)

	case *types.Interface:
		buf.WriteString("interface{")
		methods := t.MethodSet()
		for i := 0; i < methods.Len(); i++ {
			m := methods.At(i).Obj()
			if i > 0 {
				buf.WriteString("; ")
			}
			buf.WriteString(m.Name())
			ts.writeSignature(buf, m.Type().(*types.Signature), unique)
		}
		buf.WriteByte('}')

	case *types.Map:
		buf.WriteString("map[")
		ts.writeType(buf, t.Key(), unique)
		buf.WriteByte(']')
		ts.writeType(buf, t.Elem(), unique)

	case *types.Chan:
		var s string
		switch t.Dir() {
		case ast.SEND:
			s = "chan<- "
		case ast.RECV:
			s = "<-chan "
		default:
			s = "chan "
		}
		buf.WriteString(s)
		ts.writeType(buf, t.Elem(), unique)

	case *types.Named:
		obj := t.Obj()
		if pkg := obj.Pkg(); pkg != nil && pkg.Path() != "" {
			buf.WriteString(pkg.Path())
			buf.WriteByte('.')
		}
		buf.WriteString(obj.Name())

		// pkg.name may exist in multiple scopes within a package,
		// so we must add the object's pointer (which is unique)
		// to differentiate them.
		//
		// XXX Note that this is only to support the use of TypeString
		// for generating unique keys in typemap. When go/types/typemap
		// is ready, we should use that instead.
		if unique {
			buf.WriteByte('@')
			buf.WriteString(fmt.Sprintf("%p", obj))
		}

	default:
		fmt.Fprintf(buf, "<type %T>", t)
	}
}
