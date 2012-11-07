// Modifications copyright 2012 Andrew Wilkins <axwalk@gmail.com>.

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// FILE UNDER CONSTRUCTION. ANY AND ALL PARTS MAY CHANGE.
// This file implements the universe and unsafe package scopes.

package types

import "go/ast"

var (
	scope    *ast.Scope // current scope to use for initialization
	Universe *ast.Scope
	Unsafe   *ast.Object // package unsafe
	Nil      *ast.Object
)

func define(kind ast.ObjKind, name string) *ast.Object {
	obj := ast.NewObj(kind, name)
	if scope.Insert(obj) != nil {
		panic("types internal error: double declaration")
	}
	return obj
}

func defType(name string, kind BasicTypeKind) *Name {
	obj := define(ast.Typ, name)
	typ := &Name{Underlying: &Basic{Kind: kind}, Obj: obj}
	obj.Type = typ
	return typ
}

func defConst(name string) *ast.Object {
	obj := define(ast.Con, name)
	return obj // TODO(gri) fill in other properties
}

func defFun(name string) *ast.Object {
	obj := define(ast.Fun, name)
	obj.Type = &Func{}
	return obj
}

var (
	Uint,
	Uint8,
	Uint16,
	Uint32,
	Uint64,
	Int,
	Int8,
	Int16,
	Int32,
	Int64,
	Float32,
	Float64,
	Complex64,
	Complex128,
	Byte,
	Bool,
	Uintptr,
	Rune,
	UnsafePointer,
	String *Name
)

func init() {
	scope = ast.NewScope(nil)
	Universe = scope

	Uint = defType("uint", UintKind)
	Uint8 = defType("uint8", Uint8Kind)
	Uint16 = defType("uint16", Uint16Kind)
	Uint32 = defType("uint32", Uint32Kind)
	Uint64 = defType("uint64", Uint64Kind)
	Int = defType("int", IntKind)
	Int8 = defType("int8", Int8Kind)
	Int16 = defType("int16", Int16Kind)
	Int32 = defType("int32", Int32Kind)
	Int64 = defType("int64", Int64Kind)
	Float32 = defType("float32", Float32Kind)
	Float64 = defType("float64", Float64Kind)
	Complex64 = defType("complex64", Complex64Kind)
	Complex128 = defType("complex128", Complex128Kind)
	Byte = defType("byte", Uint8Kind)
	Bool = defType("bool", BoolKind)
	Uintptr = defType("uintptr", UintptrKind)
	Rune = defType("rune", Int32Kind)
	String = defType("string", StringKind)

	// type error interface {Error() string}
	obj := define(ast.Typ, "error")
	errorMethod := ast.NewObj(ast.Fun, "Error")
	errorMethod.Type = &Func{Results: ObjList([]*ast.Object{String.Obj})}
	Error := &Name{Underlying: &Interface{
		Methods: ObjList([]*ast.Object{errorMethod})}, Obj: obj}
	obj.Type = Error

	true_ := defConst("true")
	true_.Data = Const{true}
	true_.Type = Bool.Underlying
	false_ := defConst("false")
	false_.Data = Const{false}
	false_.Type = Bool.Underlying

	defConst("iota").Type = Int.Underlying

	Nil = defConst("nil")

	defFun("append")
	defFun("cap")
	defFun("close")
	defFun("complex")
	defFun("copy")
	defFun("delete")
	defFun("imag")
	defFun("len")
	defFun("make")
	defFun("new")
	defFun("panic")
	defFun("print")
	defFun("println")
	defFun("real")
	defFun("recover")

	scope = ast.NewScope(nil)
	Unsafe = ast.NewObj(ast.Pkg, "unsafe")
	Unsafe.Data = scope

	UnsafePointer = defType("Pointer", UnsafePointerKind)
	UnsafePointer.Package = "unsafe"

	uintptrResult := ast.NewObj(ast.Var, "_")
	uintptrResult.Type = Int
	alignof := defFun("Alignof").Type.(*Func)
	alignof.Results = append(alignof.Results, uintptrResult)

	offsetof := defFun("Offsetof").Type.(*Func)
	offsetof.Results = alignof.Results

	sizeof := defFun("Sizeof").Type.(*Func)
	sizeof.Results = alignof.Results
}

// vim: set ft=go :
