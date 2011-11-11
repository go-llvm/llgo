/*
Copyright (c) 2011 Andrew Wilkins <axwalk@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package main

import (
    "fmt"
    "go/ast"
    "reflect"
    "github.com/axw/gollvm/llvm"
)

func (self *Visitor) IdentGetType(ident *ast.Ident) llvm.Type {
    switch ident.Name {
        case "bool": return llvm.Int1Type()

        // TODO do we use 'metadata' to mark a type as un/signed?
        case "uint": fallthrough
        case "int": return llvm.Int32Type() // TODO 32/64 depending on arch

        case "byte": fallthrough
        case "uint8": fallthrough
        case "int8": return llvm.Int8Type()

        case "uint16": fallthrough
        case "int16": return llvm.Int16Type()

        case "uint32": fallthrough
        case "int32": return llvm.Int32Type()

        case "uint64": fallthrough
        case "int64": return llvm.Int64Type()

        case "float32": return llvm.FloatType()
        case "float64": return llvm.DoubleType()

        //case "complex64": fallthrough
        //case "complex128": fallthrough
    }

    // Resolve the object to a type.
    obj := ident.Obj
    if obj != nil {
        type_, istype := (obj.Data).(llvm.Type)
        if !istype {
            switch x := (obj.Decl).(type) {
            case *ast.TypeSpec:
                self.VisitTypeSpec(x)
                type_, istype = (obj.Data).(llvm.Type)
            default: panic("Unhandled type")
            }
        }
        if istype {return type_}
    }

    panic("Failed to resolve type: " + ident.Name)
}

func (self *Visitor) GetType(expr ast.Expr) llvm.Type {
    switch x := (expr).(type) {
    case *ast.Ident:
        return self.IdentGetType(x)
    case *ast.FuncType:
        type_ := self.VisitFuncType(x)
        type_ = llvm.PointerType(type_, 0)
        return type_
    case *ast.ArrayType:
        var len_ int = -1
        if x.Len == nil {panic("Unhandled slice ArrayType")}
        elttype := self.GetType(x.Elt)
        _, isellipsis := (x.Len).(*ast.Ellipsis)
        if !isellipsis {
            lenvalue := self.VisitExpr(x.Len)
            if lenvalue.IsAConstantInt().IsNil() {
                panic("Array length must be a constant integer expression")
            }
            len_ = int(lenvalue.ZExtValue())
        }
        return llvm.ArrayType(elttype, len_)
    case *ast.StructType:
        type_ := self.VisitStructType(x)
        return type_
    default:
        panic(fmt.Sprint("Unhandled Expr: ", reflect.TypeOf(x)))
    }
    return llvm.Type{nil}
}

func (self *Visitor) VisitFuncType(f *ast.FuncType) llvm.Type {
    var fn_args []llvm.Type = nil
    var fn_rettype llvm.Type

    // TODO process args

    if f.Results == nil || f.Results.List == nil {
        fn_rettype = llvm.VoidType()
    } else {
        for i := 0; i < len(f.Results.List); i++ {
            fn_rettype = self.GetType(f.Results.List[i].Type)
        }
    }
    return llvm.FunctionType(fn_rettype, fn_args, false)
}

func (self *Visitor) VisitStructType(s *ast.StructType) llvm.Type {
    var elttypes []llvm.Type
    var names map[string]int
    if s.Fields != nil && s.Fields.List != nil {
        elttypes = []llvm.Type{}
        names = make(map[string]int, len(s.Fields.List))
        var i int = 0
        for _, field := range s.Fields.List {
            // TODO handle field tag
            fieldtype := self.GetType(field.Type)
            if field.Names != nil {
                fieldtypes := make([]llvm.Type, len(field.Names))
                for j, name := range field.Names {
                    fieldtypes[j] = fieldtype
                    names[name.String()] = i+j
                }
                elttypes = append(elttypes, fieldtypes...)
                i += len(field.Names)
            } else {
                elttypes = append(elttypes, fieldtype)
                i++
            }
        }
    }
    type_ := llvm.StructType(elttypes, false)

    // Add a mapping from type to a slice of names.
    self.typefields[type_.C] = names

    // TODO record the names as a global constant array. Then any
    // values of this struct type will have a metadata node attached
    // to associate with the name array. It would be nice if we could
    // just attach metadata to types.

    return type_
}

// vim: set ft=go :

