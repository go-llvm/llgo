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
    "sort"
    "reflect"
)

type TypeInfo struct {
    methods map[string]*ast.Object
    ptrmethods map[string]*ast.Object
}

type TypeMap map[Type]*TypeInfo

func (m *TypeMap) lookup(t Type) *TypeInfo {
    info := (*m)[t]
    if info == nil {
        info = new(TypeInfo)
        info.methods = make(map[string]*ast.Object)
        info.ptrmethods = make(map[string]*ast.Object)
        (*m)[t] = info
    }
    return info
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
                ident := f.Params.List[i].Names[j]
                if ident != nil {
                    args[j] = ident.Obj
                } else {
                    args[j] = ast.NewObj(ast.Var, "_")
                }
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
                    ident := f.Results.List[i].Names[j]
                    if ident != nil {
                        results[j] = ident.Obj
                    } else {
                        results[j] = ast.NewObj(ast.Var, "_")
                    }
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
    var typ *Struct = new(Struct)
    if s.Fields != nil && s.Fields.List != nil {
        tags := make(map[*ast.Object]string)
        var i int = 0
        for _, field := range s.Fields.List {
            fieldtype := c.GetType(field.Type)
            if field.Names != nil {
                for _, name := range field.Names {
                    obj := name.Obj
                    if obj == nil {obj = ast.NewObj(ast.Var, "_")}
                    obj.Type = fieldtype
                    typ.Fields = append(typ.Fields, obj)
                    if field.Tag != nil {tags[obj] = field.Tag.Value}
                }
                i += len(field.Names)
            } else {
                obj := ast.NewObj(ast.Var, "_")
                obj.Type = fieldtype
                typ.Fields = append(typ.Fields, obj)
                if field.Tag != nil {tags[obj] = field.Tag.Value}
                i++
            }
        }

        sort.Sort(typ.Fields)
        typ.Tags = make([]string, len(typ.Fields))
        for i, field := range typ.Fields {
            // TODO unquote string?
            typ.Tags[i] = tags[field]
        }
    }
    return typ
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

