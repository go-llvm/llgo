/*
Copyright (c) 2011, 2012 Andrew Wilkins <axwalk@gmail.com>

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

package llgo

import (
    "big"
    "fmt"
    "go/ast"
    "reflect"
)

// A struct for maintaining information about types. Types are persisted in the
// LLVM bitcode, and store information about struct fields and methods.
//
// XXX persistence isn't implemented yet
type TypeInfo struct {
    Methods      map[string]*ast.Object
    FieldIndexes map[string]int
}

var (
    UintType Type = &Basic{Kind: Uint}
    Uint8Type Type = &Basic{Kind: Uint8}
    Uint16Type Type = &Basic{Kind: Uint16}
    Uint32Type Type = &Basic{Kind: Uint32}
    Uint64Type Type = &Basic{Kind: Uint64}

    IntType Type = &Basic{Kind: Int}
    Int8Type Type = &Basic{Kind: Int8}
    Int16Type Type = &Basic{Kind: Int16}
    Int32Type Type = &Basic{Kind: Int32}
    Int64Type Type = &Basic{Kind: Int64}

    Float32Type Type = &Basic{Kind: Float32}
    Float64Type Type = &Basic{Kind: Float64}
    Complex64Type Type = &Basic{Kind: Complex64}
    Complex128Type Type = &Basic{Kind: Complex128}

    ByteType Type = &Basic{Kind: Byte}
    BoolType Type = &Basic{Kind: Bool}
)

// Look up a method by name. If the func declaration for the method has not yet
// been processed, then we'll look it up in the package scope wherein the type
// was defined.
func (t *TypeInfo) MethodByName(name string) *ast.Object {
    if t.Methods != nil {
        return t.Methods[name]
    }
    return nil
}

func (t *TypeInfo) FieldIndex(name string) (i int, exists bool) {
    i, exists = t.FieldIndexes[name]
    return
}

// Get a Type from an identifier.
func (c *compiler) IdentGetType(ident *ast.Ident) Type {
    switch ident.Name {
        case "bool": return BoolType
        case "byte": return ByteType

        case "uint": return UintType
        case "uint8": return Uint8Type
        case "uint16": return Uint16Type
        case "uint32": return Uint32Type
        case "uint64": return Uint64Type

        case "int": return IntType
        case "int8": return Int8Type
        case "int16": return Int16Type
        case "int32": return Int32Type
        case "int64": return Int64Type

        case "float32": return Float32Type
        case "float64": return Float64Type

        case "complex64": return Complex64Type
        case "complex128": return Complex128Type
    }
    return c.ObjGetType(ident.Obj)
}

// Get a Type from an ast object.
func (c *compiler) ObjGetType(obj *ast.Object) Type {
    if obj != nil {
        type_, istype := (obj.Type).(Type)
        if !istype {
            switch x := (obj.Decl).(type) {
            case *ast.TypeSpec:
                c.VisitTypeSpec(x)
                type_, istype = (obj.Type).(Type)
            }
        }
        if istype {return type_}
    }
    return nil
}

func (c *compiler) GetType(expr ast.Expr) Type {
    switch x := (expr).(type) {
    case *ast.Ident:
        return c.IdentGetType(x)
    case *ast.FuncType:
        fn_type := c.VisitFuncType(x)
        return &Pointer{Base: fn_type}
    case *ast.ArrayType:
        elttype := c.GetType(x.Elt)
        if x.Len == nil {
            return &Slice{Elt: elttype}
        } else {
            result := &Array{Elt: elttype}
            _, isellipsis := (x.Len).(*ast.Ellipsis)
            if !isellipsis {
                lenvalue := c.VisitExpr(x.Len)
                constval, isconst := lenvalue.(ConstValue)
                if !isconst {
                    panic("Array length must be a constant integer expression")
                }
                intval, isint := (constval.val).(*big.Int)
                if !isint {
                    panic("Array length must be a constant integer expression")
                }
                result.Len = uint64(intval.Int64())
            }
            return result
        }
    case *ast.StructType:
        return c.VisitStructType(x)
    case *ast.InterfaceType:
        return c.VisitInterfaceType(x)
    case *ast.StarExpr:
        return &Pointer{Base: c.GetType(x.X)}
    default:
        panic(fmt.Sprint("Unhandled Expr: ", reflect.TypeOf(x)))
    }
    return nil
}

func (c *compiler) VisitFuncType(f *ast.FuncType) *Func {
    var fn_type Func

    if f.Params != nil {
        for i := 0; i < len(f.Params.List); i++ {
            namecount := len(f.Params.List[i].Names)
            args := make([]*ast.Object, namecount)
            typ := c.GetType(f.Params.List[i].Type)
            for j := 0; j < namecount; j++ {
                name := "_"
                ident := f.Params.List[i].Names[j]
                if ident != nil {name = ident.String()}
                args[j] = ast.NewObj(ast.Var, name)
                args[j].Type = typ
            }
            fn_type.Params = append(fn_type.Params, args...)
        }
    }

    if f.Results != nil {
        for i := 0; i < len(f.Results.List); i++ {
            namecount := len(f.Results.List[i].Names)
            typ := c.GetType(f.Results.List[i].Type)
            if namecount > 0 {
                results := make([]*ast.Object, namecount)
                for j := 0; j < namecount; j++ {
                    name := "_"
                    ident := f.Results.List[i].Names[j]
                    if ident != nil {name = ident.String()}
                    results[j] = ast.NewObj(ast.Var, name)
                    results[j].Type = typ
                }
                fn_type.Results = append(fn_type.Results, results...)
            } else {
                result := ast.NewObj(ast.Var, "_")
                result.Type = typ
                fn_type.Results = append(fn_type.Results, result)
            }
        }
    }

    return &fn_type
}

func (c *compiler) VisitStructType(s *ast.StructType) *Struct {
    var typ Struct
    if s.Fields != nil && s.Fields.List != nil {
        var i int = 0
        for _, field := range s.Fields.List {
            // TODO handle field tag
            fieldtype := c.GetType(field.Type)
            if field.Names != nil {
                //fieldtypes := make([]*ast.Object, len(field.Names))
                for _, name := range field.Names {
                    obj := ast.NewObj(ast.Var, name.String())
                    obj.Type = typ
                    typ.Fields = append(typ.Fields, obj)
                    if field.Tag != nil {
                        // TODO unquote string?
                        typ.Tags = append(typ.Tags, field.Tag.Value)
                    } else {
                        typ.Tags = append(typ.Tags, "")
                    }
                }
                i += len(field.Names)
            } else {
                obj := ast.NewObj(ast.Var, "_")
                obj.Type = fieldtype
                typ.Fields = append(typ.Fields, obj)
                if field.Tag != nil {
                    // TODO unquote string?
                    typ.Tags = append(typ.Tags, field.Tag.Value)
                } else {
                    typ.Tags = append(typ.Tags, "")
                }
                i++
            }
        }
    }
    return &typ
}

func (c *compiler) VisitInterfaceType(i *ast.InterfaceType) *Interface {
    var iface Interface
    if i.Methods != nil && i.Methods.List != nil {
        for _, field := range i.Methods.List {
            if field.Names == nil {
                // If field.Names is nil, then we have an embedded interface.
                fmt.Println("==nil")
                embedded := c.GetType(field.Type)

                embedded_iface, isiface := embedded.(*Interface)
                if isiface {
                    iface.Methods = append(iface.Methods,
                                           embedded_iface.Methods...)
                }
            } else {
                // TODO
                fmt.Println("!=nil")
            }
        }
    }
    return &iface
}

// vim: set ft=go :

