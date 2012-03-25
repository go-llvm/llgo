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
	"github.com/axw/gollvm/llvm"
	"go/ast"
	"sort"
)

type BasicTypeKind int

// Constants for basic types.
const (
	NilKind BasicTypeKind = iota
	Uint8Kind
	Uint16Kind
	Uint32Kind
	Uint64Kind
	Int8Kind
	Int16Kind
	Int32Kind
	Int64Kind
	Float32Kind
	Float64Kind
	Complex64Kind
	Complex128Kind
	UntypedIntKind
	UntypedFloatKind
	UntypedComplexKind
	ByteKind
	BoolKind
	UintptrKind
	RuneKind
	StringKind
)

// All types implement the Type interface.
type Type interface {
	LLVMType() llvm.Type
	String() string
}

// A Bad type is a non-nil placeholder type when we don't know a type.
type Bad struct {
	Msg string // for better error reporting/debugging
}

func (b *Bad) String() string {
	return fmt.Sprint("Bad(", b.Msg, ")")
}

func (b *Bad) LLVMType() llvm.Type {
	return llvm.Type{nil}
}

// A Basic represents a (unnamed) basic type.
type Basic struct {
	Kind BasicTypeKind
	// TODO(gri) need a field specifying the exact basic type
}

func (b *Basic) String() string {
	return fmt.Sprint("Basic(", b.Kind, ")")
}

func (b *Basic) LLVMType() llvm.Type {
	switch b.Kind {
	case BoolKind:
		return llvm.Int1Type()
	case ByteKind, Int8Kind, Uint8Kind:
		return llvm.Int8Type()
	case Int16Kind, Uint16Kind:
		return llvm.Int16Type()
	case UintptrKind, Int32Kind, Uint32Kind:
		return llvm.Int32Type()
	case Int64Kind, Uint64Kind:
		return llvm.Int64Type()
	case Float32Kind:
		return llvm.FloatType()
	case Float64Kind:
		return llvm.DoubleType()
	//case Complex64: TODO
	//case Complex128:
	//case UntypedInt:
	//case UntypedFloat:
	//case UntypedComplex:
	case StringKind:
		i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
		elements := []llvm.Type{i8ptr, llvm.Int32Type()}
		return llvm.StructType(elements, false)
	case RuneKind:
		return Int.LLVMType()
	}
	panic(fmt.Sprint("unhandled kind: ", b.Kind))
}

// An Array represents an array type [Len]Elt.
type Array struct {
	Len uint64
	Elt Type
}

func (a *Array) String() string {
	return fmt.Sprint("Array(", a.Len, ", ", a.Elt, ")")
}

func (a *Array) LLVMType() llvm.Type {
	return llvm.ArrayType(a.Elt.LLVMType(), int(a.Len))
}

// A Slice represents a slice type []Elt.
type Slice struct {
	Elt Type
}

func (s *Slice) LLVMType() llvm.Type {
	elements := []llvm.Type{
		llvm.PointerType(s.Elt.LLVMType(), 0),
		llvm.Int32Type(),
		llvm.Int32Type(),
	}
	return llvm.StructType(elements, false)
}

func (s *Slice) String() string {
	return fmt.Sprint("Slice(", s.Elt, ")")
}

// A Struct represents a struct type struct{...}.
// Anonymous fields are represented by objects with empty names.
type Struct struct {
	Fields ObjList  // struct fields; or nil
	Tags   []string // corresponding tags; or nil
	typ    llvm.Type
	// TODO(gri) This type needs some rethinking:
	// - at the moment anonymous fields are marked with "" object names,
	//   and their names have to be reconstructed
	// - there is no scope for fast lookup (but the parser creates one)
}

func (s *Struct) String() string {
	return fmt.Sprint("Struct(", s.Fields, ", ", s.Tags, ")")
}

func (s *Struct) LLVMType() llvm.Type {
	if s.typ.IsNil() {
		s.typ = llvm.GlobalContext().StructCreateNamed("")
		elements := make([]llvm.Type, len(s.Fields))
		for i, f := range s.Fields {
			ft := f.Type.(Type)
			elements[i] = ft.LLVMType()
		}
		//return llvm.StructType(elements, false)
		s.typ.StructSetBody(elements, false)
	}
	return s.typ
}

// A Pointer represents a pointer type *Base.
type Pointer struct {
	Base Type
}

func (p *Pointer) String() string {
	return fmt.Sprint("Pointer(", p.Base, ")")
}

func (p *Pointer) LLVMType() llvm.Type {
	return llvm.PointerType(p.Base.LLVMType(), 0)
}

// A Func represents a function type func(...) (...).
// Unnamed parameters are represented by objects with empty names.
type Func struct {
	Recv       *ast.Object // nil if not a method
	Params     ObjList     // (incoming) parameters from left to right; or nil
	Results    ObjList     // (outgoing) results from left to right; or nil
	IsVariadic bool        // true if the last parameter's type is of the form ...T
}

func (f *Func) String() string {
	return fmt.Sprintf("Func(%v, %v, %v, %v)", f.Recv,
		f.Params, f.Results, f.IsVariadic)
}

func (f *Func) LLVMType() llvm.Type {
	param_types := make([]llvm.Type, 0)

	// Add receiver parameter.
	if f.Recv != nil {
		recv_type := f.Recv.Type.(Type)
		param_types = append(param_types, recv_type.LLVMType())
	}

	for i, param := range f.Params {
		param_type := param.Type.(Type)
		if f.IsVariadic && i == len(f.Params)-1 {
			param_type = &Slice{Elt: param_type}
		}
		param_types = append(param_types, param_type.LLVMType())
	}

	var return_type llvm.Type
	switch len(f.Results) {
	case 0:
		return_type = llvm.VoidType()
	case 1:
		return_type = (f.Results[0].Type.(Type)).LLVMType()
	default:
		elements := make([]llvm.Type, len(f.Results))
		for i, result := range f.Results {
			elements[i] = result.Type.(Type).LLVMType()
		}
		return_type = llvm.StructType(elements, false)
	}

	fn_type := llvm.FunctionType(return_type, param_types, false)
	return llvm.PointerType(fn_type, 0)

	/*
	   if fn_name == "init" {
	       // Make init functions anonymous
	       fn_name = ""
	   } else if f.Recv != nil {
	       return_type := llvm.VoidType() //fn_type.ReturnType() TODO
	       param_types := make([]llvm.Type, 0) //fn_type.ParamTypes()
	       isvararg := fn_type.IsVariadic

	       // Add receiver as the first parameter type.
	       recv_type := c.GetType(f.Recv.List[0].Type)
	       if recv_type != nil {
	           param_types = append(param_types, recv_type.LLVMType())
	       }

	       for _, param := range fn_type.Params {
	           param_type := param.Type.(Type)
	           param_types = append(param_types, param_type.LLVMType())
	       }

	       fn_type = llvm.FunctionType(return_type, param_types, isvararg)
	   }

	   return fn_type
	*/
}

// An Interface represents an interface type interface{...}.
type Interface struct {
	Methods ObjList // interface methods sorted by name; or nil
}

func (i *Interface) String() string {
	return fmt.Sprint("Interface(", i.Methods, ")")
}

func (i *Interface) LLVMType() llvm.Type {
	ptr_type := llvm.PointerType(llvm.Int8Type(), 0)
	elements := make([]llvm.Type, 1+len(i.Methods))
	elements[0] = ptr_type // value
	for n, m := range i.Methods {
		fnptr := Pointer{Base: m.Type.(Type)}
		elements[n+1] = fnptr.LLVMType()
	}
	return llvm.StructType(elements, false)
}

// A Map represents a map type map[Key]Elt.
type Map struct {
	Key, Elt Type
}

func (m *Map) String() string {
	return fmt.Sprint("Map(", m.Key, ", ", m.Elt, ")")
}

func (m *Map) LLVMType() llvm.Type {
	return llvm.Type{nil} // TODO
}

// A Chan represents a channel type chan Elt, <-chan Elt, or chan<-Elt.
type Chan struct {
	Dir ast.ChanDir
	Elt Type
}

func (c *Chan) String() string {
	return fmt.Sprint("Chan(", c.Dir, ", ", c.Elt, ")")
}

func (c *Chan) LLVMType() llvm.Type {
	return llvm.Type{nil} // TODO
}

// A Name represents a named type as declared in a type declaration.
type Name struct {
	Underlying Type        // nil if not fully declared
	Obj        *ast.Object // corresponding declared object
	Methods    ObjList
	// TODO(gri) need to remember fields and methods.
	visited bool
}

func (n *Name) String() string {
	if !n.visited {
		n.visited = true
		res := fmt.Sprint("Name(", n.Obj.Name, ", ", n.Underlying, ")")
		n.visited = false
		return res
	}
	return fmt.Sprint("Name(", n.Obj.Name, ", ...)")
}

func (n *Name) LLVMType() llvm.Type {
	return n.Underlying.LLVMType()
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
