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
	"fmt"
	"github.com/axw/llgo/types"
	"go/ast"
	"math/big"
	"sort"
)

// Get a Type from an ast object.
func (c *compiler) ObjGetType(obj *ast.Object) types.Type {
	if obj != nil {
		switch type_ := obj.Type.(type) {
		case types.Type:
			return type_
		}

		switch x := (obj.Decl).(type) {
		case *ast.TypeSpec:
			c.VisitTypeSpec(x)
			type_, _ := (obj.Type).(types.Type)
			return type_
		}
	}
	return nil
}

func (c *compiler) GetType(expr ast.Expr) types.Type {
	switch x := (expr).(type) {
	case *ast.Ident:
		obj := c.LookupObj(x.Name)
		return c.ObjGetType(obj)
	case *ast.FuncType:
		return c.VisitFuncType(x)
	case *ast.ArrayType:
		elttype := c.GetType(x.Elt)
		if x.Len == nil {
			return &types.Slice{Elt: elttype}
		} else {
			result := &types.Array{Elt: elttype}
			_, isellipsis := (x.Len).(*ast.Ellipsis)
			if !isellipsis {
				lenvalue := c.VisitExpr(x.Len)
				constval, isconst := lenvalue.(ConstValue)
				if !isconst {
					panic("Array length must be a constant integer expression")
				}
				intval, isint := (constval.Val).(*big.Int)
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
		return &types.Pointer{Base: c.GetType(x.X)}
	case *ast.Ellipsis:
		return c.GetType(x.Elt)
	default:
		value := c.VisitExpr(expr)
		return value.(TypeValue).typ
	}
	return nil
}

func (c *compiler) VisitFuncType(f *ast.FuncType) *types.Func {
	var fn_type types.Func

	if f.Params != nil && len(f.Params.List) > 0 {
		final_param_type := f.Params.List[len(f.Params.List)-1].Type
		if _, varargs := final_param_type.(*ast.Ellipsis); varargs {
			fn_type.IsVariadic = true
		}
		for i := 0; i < len(f.Params.List); i++ {
			namecount := len(f.Params.List[i].Names)
			typ := c.GetType(f.Params.List[i].Type)
			if namecount == 0 {
				arg := ast.NewObj(ast.Var, "_")
				arg.Type = typ
				fn_type.Params = append(fn_type.Params, arg)
			} else {
				args := make([]*ast.Object, namecount)
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

func (c *compiler) VisitStructType(s *ast.StructType) *types.Struct {
	var typ = new(types.Struct)
	if s.Fields != nil && s.Fields.List != nil {
		tags := make(map[*ast.Object]string)
		var i int = 0
		for _, field := range s.Fields.List {
			fieldtype := c.GetType(field.Type)
			if field.Names != nil {
				for _, name := range field.Names {
					obj := name.Obj
					if obj == nil {
						obj = ast.NewObj(ast.Var, "_")
					}
					obj.Type = fieldtype
					typ.Fields = append(typ.Fields, obj)
					if field.Tag != nil {
						tags[obj] = field.Tag.Value
					}
				}
				i += len(field.Names)
			} else {
				obj := ast.NewObj(ast.Var, "_")
				obj.Type = fieldtype
				typ.Fields = append(typ.Fields, obj)
				if field.Tag != nil {
					tags[obj] = field.Tag.Value
				}
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

func (c *compiler) VisitInterfaceType(i *ast.InterfaceType) *types.Interface {
	var iface types.Interface
	if i.Methods != nil && i.Methods.List != nil {
		for _, field := range i.Methods.List {
			if field.Names == nil {
				// If field.Names is nil, then we have an embedded interface.
				fmt.Println("==nil")
				embedded := c.GetType(field.Type)

				embedded_iface, isiface := embedded.(*types.Interface)
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
