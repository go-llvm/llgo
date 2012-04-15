// Modifications copyright 2011, 2012 Andrew Wilkins <axwalk@gmail.com>.

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// PACKAGE UNDER CONSTRUCTION. ANY AND ALL PARTS MAY CHANGE.
// Package types declares the types used to represent Go types.
//

package types

import (
	"fmt"
	"go/ast"
	"reflect"
	"sort"
)

type BasicTypeKind reflect.Kind

// Constants for basic types.
const (
	BoolKind = BasicTypeKind(reflect.Bool)
	IntKind = BasicTypeKind(reflect.Int)
	Int8Kind = BasicTypeKind(reflect.Int8)
	Int16Kind = BasicTypeKind(reflect.Int16)
	Int32Kind = BasicTypeKind(reflect.Int32)
	Int64Kind = BasicTypeKind(reflect.Int64)
	UintKind = BasicTypeKind(reflect.Uint)
	Uint8Kind = BasicTypeKind(reflect.Uint8)
	Uint16Kind = BasicTypeKind(reflect.Uint16)
	Uint32Kind = BasicTypeKind(reflect.Uint32)
	Uint64Kind = BasicTypeKind(reflect.Uint64)
	UintptrKind = BasicTypeKind(reflect.Uintptr)
	Float32Kind = BasicTypeKind(reflect.Float32)
	Float64Kind = BasicTypeKind(reflect.Float64)
	Complex64Kind = BasicTypeKind(reflect.Complex64)
	Complex128Kind = BasicTypeKind(reflect.Complex128)
	StringKind = BasicTypeKind(reflect.String)

	NilKind BasicTypeKind = StringKind + iota + 1
	RuneKind
	ByteKind
	UntypedIntKind
	UntypedFloatKind
	UntypedComplexKind
)

// All types implement the Type interface.
type Type interface {
	isType()
	String() string
}

// All concrete types embed ImplementsType which
// ensures that all types implement the Type interface.
type ImplementsType struct{}

func (t *ImplementsType) isType() {}

// A Bad type is a non-nil placeholder type when we don't know a type.
type Bad struct {
	ImplementsType
	Msg string // for better error reporting/debugging
}

func (b *Bad) String() string {
	return fmt.Sprint("Bad(", b.Msg, ")")
}

// A Basic represents a (unnamed) basic type.
type Basic struct {
	ImplementsType
	Kind BasicTypeKind
	// TODO(gri) need a field specifying the exact basic type
}

func (b *Basic) String() string {
	return fmt.Sprint("Basic(", b.Kind, ")")
}

// An Array represents an array type [Len]Elt.
type Array struct {
	ImplementsType
	Len uint64
	Elt Type
}

func (a *Array) String() string {
	return fmt.Sprint("Array(", a.Len, ", ", a.Elt, ")")
}

// A Slice represents a slice type []Elt.
type Slice struct {
	ImplementsType
	Elt Type
}

func (s *Slice) String() string {
	return fmt.Sprint("Slice(", s.Elt, ")")
}

// A Struct represents a struct type struct{...}.
// Anonymous fields are represented by objects with empty names.
type Struct struct {
	ImplementsType
	Fields ObjList  // struct fields; or nil
	Tags   []string // corresponding tags; or nil
	// TODO(gri) This type needs some rethinking:
	// - at the moment anonymous fields are marked with "" object names,
	//   and their names have to be reconstructed
	// - there is no scope for fast lookup (but the parser creates one)
}

func (s *Struct) String() string {
	return fmt.Sprint("Struct(", s.Fields, ", ", s.Tags, ")")
}

// A Pointer represents a pointer type *Base.
type Pointer struct {
	ImplementsType
	Base Type
}

func (p *Pointer) String() string {
	return fmt.Sprint("Pointer(", p.Base, ")")
}

// A Func represents a function type func(...) (...).
// Unnamed parameters are represented by objects with empty names.
type Func struct {
	ImplementsType
	Recv       *ast.Object // nil if not a method
	Params     ObjList     // (incoming) parameters from left to right; or nil
	Results    ObjList     // (outgoing) results from left to right; or nil
	IsVariadic bool        // true if the last parameter's type is of the form ...T
}

func (f *Func) String() string {
	return fmt.Sprintf("Func(%v, %v, %v, %v)", f.Recv,
		f.Params, f.Results, f.IsVariadic)
}

// An Interface represents an interface type interface{...}.
type Interface struct {
	ImplementsType
	Methods ObjList // interface methods sorted by name; or nil
}

func (i *Interface) String() string {
	return fmt.Sprint("Interface(", i.Methods, ")")
}

// A Map represents a map type map[Key]Elt.
type Map struct {
	ImplementsType
	Key, Elt Type
}

func (m *Map) String() string {
	return fmt.Sprint("Map(", m.Key, ", ", m.Elt, ")")
}

// A Chan represents a channel type chan Elt, <-chan Elt, or chan<-Elt.
type Chan struct {
	ImplementsType
	Dir ast.ChanDir
	Elt Type
}

func (c *Chan) String() string {
	return fmt.Sprint("Chan(", c.Dir, ", ", c.Elt, ")")
}

// A Name represents a named type as declared in a type declaration.
type Name struct {
	ImplementsType
	Underlying Type        // nil if not fully declared
	Obj        *ast.Object // corresponding declared object
	Methods    ObjList
	// TODO(gri) need to remember fields and methods.
}

func (n *Name) String() string {
	u := n.Underlying
	if u != nil {
		n.Underlying = nil
		res := fmt.Sprint("Name(", n.Obj.Name, ", ", u, ")")
		n.Underlying = u
		return res
	}
	return fmt.Sprint("Name(", n.Obj.Name, ", ...)")
}

// typ must be a pointer type; Deref returns the pointer's base type.
func Deref(typ Type) Type {
	return typ.(*Pointer).Base
}

// Underlying returns the underlying type of a type.
func Underlying(typ Type) Type {
	if typ, ok := typ.(*Name); ok {
		utyp := typ.Underlying
		if _, ok := utyp.(*Basic); !ok {
			return utyp
		}
		// the underlying type of a type name referring
		// to an (untyped) basic type is the basic type
		// name
	}
	return typ
}

// An ObjList represents an ordered (in some fashion) list of objects.
type ObjList []*ast.Object

// ObjList implements sort.Interface.
func (list ObjList) Len() int           { return len(list) }
func (list ObjList) Less(i, j int) bool { return list[i].Name < list[j].Name }
func (list ObjList) Swap(i, j int)      { list[i], list[j] = list[j], list[i] }

// Sort sorts an object list by object name.
func (list ObjList) Sort() { sort.Sort(list) }

func (list ObjList) String() string {
	s := "["
	for _, o := range list {
		s += fmt.Sprint(o)
	}
	return s + "]"
}

// identicalTypes returns true if both lists a and b have the
// same length and corresponding objects have identical types.
func identicalTypes(a, b ObjList) bool {
	if len(a) == len(b) {
		for i, x := range a {
			y := b[i]
			if !Identical(x.Type.(Type), y.Type.(Type)) {
				return false
			}
		}
		return true
	}
	return false
}

// Identical returns true if two types are identical.
func Identical(x, y Type) bool {
	if x == y {
		return true
	}

	switch x := x.(type) {
	case *Bad:
		// A Bad type is always identical to any other type
		// (to avoid spurious follow-up errors).
		return true

	case *Basic:
		if y, ok := y.(*Basic); ok {
			panic("unimplemented")
			_ = y
		}

	case *Array:
		// Two array types are identical if they have identical element types
		// and the same array length.
		if y, ok := y.(*Array); ok {
			return x.Len == y.Len && Identical(x.Elt, y.Elt)
		}

	case *Slice:
		// Two slice types are identical if they have identical element types.
		if y, ok := y.(*Slice); ok {
			return Identical(x.Elt, y.Elt)
		}

	case *Struct:
		// Two struct types are identical if they have the same sequence of fields,
		// and if corresponding fields have the same names, and identical types,
		// and identical tags. Two anonymous fields are considered to have the same
		// name. Lower-case field names from different packages are always different.
		if y, ok := y.(*Struct); ok {
			// TODO(gri) handle structs from different packages
			if identicalTypes(x.Fields, y.Fields) {
				for i, f := range x.Fields {
					g := y.Fields[i]
					if f.Name != g.Name || x.Tags[i] != y.Tags[i] {
						return false
					}
				}
				return true
			}
		}

	case *Pointer:
		// Two pointer types are identical if they have identical base types.
		if y, ok := y.(*Pointer); ok {
			return Identical(x.Base, y.Base)
		}

	case *Func:
		// Two function types are identical if they have the same number of parameters
		// and result values, corresponding parameter and result types are identical,
		// and either both functions are variadic or neither is. Parameter and result
		// names are not required to match.
		if y, ok := y.(*Func); ok {
			return identicalTypes(x.Params, y.Params) &&
				identicalTypes(x.Results, y.Results) &&
				x.IsVariadic == y.IsVariadic
		}

	case *Interface:
		// Two interface types are identical if they have the same set of methods with
		// the same names and identical function types. Lower-case method names from
		// different packages are always different. The order of the methods is irrelevant.
		if y, ok := y.(*Interface); ok {
			return identicalTypes(x.Methods, y.Methods) // methods are sorted
		}

	case *Map:
		// Two map types are identical if they have identical key and value types.
		if y, ok := y.(*Map); ok {
			return Identical(x.Key, y.Key) && Identical(x.Elt, y.Elt)
		}

	case *Chan:
		// Two channel types are identical if they have identical value types
		// and the same direction.
		if y, ok := y.(*Chan); ok {
			return x.Dir == y.Dir && Identical(x.Elt, y.Elt)
		}

	case *Name:
		// Two named types are identical if their type names originate
		// in the same type declaration.
		if y, ok := y.(*Name); ok {
			return x.Obj == y.Obj ||
				// permit bad objects to be equal to avoid
				// follow up errors
				x.Obj != nil && x.Obj.Kind == ast.Bad ||
				y.Obj != nil && y.Obj.Kind == ast.Bad
		}
	}

	return false
}

// vim: set ft=go :
