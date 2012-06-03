// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements the Check function, which typechecks a package.

package types

import (
	"fmt"
	"go/ast"
	"go/scanner"
	"go/token"
	"strconv"
)

const debug = false

type checker struct {
	fset    *token.FileSet
	errors  scanner.ErrorList
	types   map[ast.Expr]Type
	methods map[*ast.Object]ObjList
}

func (c *checker) errorf(pos token.Pos, format string, args ...interface{}) string {
	msg := fmt.Sprintf(format, args...)
	c.errors.Add(c.fset.Position(pos), msg)
	return msg
}

// collectFields collects struct fields tok = token.STRUCT), interface methods
// (tok = token.INTERFACE), and function arguments/results (tok = token.FUNC).
func (c *checker) collectFields(tok token.Token, list *ast.FieldList, cycleOk bool) (fields ObjList, tags []string, isVariadic bool) {
	if list != nil {
		for _, field := range list.List {
			ftype := field.Type
			if t, ok := ftype.(*ast.Ellipsis); ok {
				ftype = t.Elt
				isVariadic = true
			}
			typ := c.makeType(ftype, cycleOk)
			tag := ""
			if field.Tag != nil {
				assert(field.Tag.Kind == token.STRING)
				tag, _ = strconv.Unquote(field.Tag.Value)
			}
			if len(field.Names) > 0 {
				// named fields
				for _, name := range field.Names {
					obj := name.Obj
					obj.Type = typ
					fields = append(fields, obj)
					if tok == token.STRUCT {
						tags = append(tags, tag)
					}
				}
			} else {
				// anonymous field
				switch tok {
				case token.STRUCT:
					tags = append(tags, tag)
					fallthrough
				case token.FUNC:
					obj := ast.NewObj(ast.Var, "")
					obj.Type = typ
					fields = append(fields, obj)
				case token.INTERFACE:
					utyp := Underlying(typ)
					if typ, ok := utyp.(*Interface); ok {
						// TODO(gri) This is not good enough. Check for double declarations!
						fields = append(fields, typ.Methods...)
					} else if _, ok := utyp.(*Bad); !ok {
						// if utyp is Bad, don't complain (the root cause was reported before)
						c.errorf(ftype.Pos(), "interface contains embedded non-interface type")
					}
				default:
					panic("unreachable")
				}
			}
		}
	}
	return
}

// collectMethods collects the method declarations from an AST File and
// returns a mapping from receiver types to their method FuncDecl's.
func (c *checker) collectMethods(file *ast.File) {
	for _, decl := range file.Decls {
		if funcdecl, ok := decl.(*ast.FuncDecl); ok && funcdecl.Recv != nil {
			recvField := funcdecl.Recv.List[0]
			var recv *ast.Ident
			switch typ := recvField.Type.(type) {
			case *ast.StarExpr:
				recv = typ.X.(*ast.Ident)
			case *ast.Ident:
				recv = typ
			default:
				panic("bad receiver type expression")
			}

			// The Obj field of the funcdecl wll be nil, so we'll have to
			// create a new one.
			funcdecl.Name.Obj = ast.NewObj(ast.Fun, funcdecl.Name.String())
			funcdecl.Name.Obj.Decl = funcdecl
			c.methods[recv.Obj] = append(c.methods[recv.Obj], funcdecl.Name.Obj)
		}
	}
}

// makeType makes a new type for an AST type specification x or returns
// the type referred to by a type name x. If cycleOk is set, a type may
// refer to itself directly or indirectly; otherwise cycles are errors.
//
func (c *checker) makeType(x ast.Expr, cycleOk bool) (typ Type) {
	if debug {
		fmt.Printf("makeType (cycleOk = %v)\n", cycleOk)
		ast.Print(c.fset, x)
		defer func() {
			fmt.Printf("-> %T %v\n\n", typ, typ)
		}()
	}

	switch t := x.(type) {
	case *ast.BadExpr:
		return &Bad{}

	case *ast.Ident:
		// type name
		obj := t.Obj
		if obj == nil {
			// unresolved identifier (error has been reported before)
			return &Bad{Msg: "unresolved identifier"}
		}
		if obj.Kind != ast.Typ {
			msg := c.errorf(t.Pos(), "%s is not a type", t.Name)
			return &Bad{Msg: msg}
		}
		c.checkObj(obj, cycleOk)
		if !cycleOk && obj.Type.(*Name).Underlying == nil {
			// TODO(gri) Enable this message again once its position
			// is independent of the underlying map implementation.
			// msg := c.errorf(obj.Pos(), "illegal cycle in declaration of %s", obj.Name)
			msg := "illegal cycle"
			return &Bad{Msg: msg}
		}
		return obj.Type.(Type)

	case *ast.ParenExpr:
		return c.makeType(t.X, cycleOk)

	case *ast.SelectorExpr:
		// qualified identifier
		// TODO (gri) eventually, this code belongs to expression
		//            type checking - here for the time being
		if ident, ok := t.X.(*ast.Ident); ok {
			if obj := ident.Obj; obj != nil {
				if obj.Kind != ast.Pkg {
					msg := c.errorf(ident.Pos(), "%s is not a package", obj.Name)
					return &Bad{Msg: msg}
				}
				pkgscope := obj.Data.(*ast.Scope)
				t.Sel.Obj = pkgscope.Lookup(t.Sel.Name)
				return c.makeType(t.Sel, cycleOk)
				// TODO(gri) we have a package name but don't
				// have the mapping from package name to package
				// scope anymore (created in ast.NewPackage).
				//return &Bad{} // for now
			}
		}
		// TODO(gri) can this really happen (the parser should have excluded this)?
		msg := c.errorf(t.Pos(), "expected qualified identifier")
		return &Bad{Msg: msg}

	case *ast.StarExpr:
		return &Pointer{Base: c.makeType(t.X, true)}

	case *ast.ArrayType:
		if t.Len != nil {
			// TODO(gri) compute length
			return &Array{Elt: c.makeType(t.Elt, cycleOk)}
		}
		return &Slice{Elt: c.makeType(t.Elt, true)}

	case *ast.StructType:
		fields, tags, _ := c.collectFields(token.STRUCT, t.Fields, cycleOk)
		indices := make(map[string]uint64)
		for i, f := range fields {
			if f.Name == "" {
				typ := f.Type
				if ptr, ok := typ.(*Pointer); ok {
					typ = ptr.Base
				}
				indices[typ.(*Name).Obj.Name] = uint64(i)
			} else {
				indices[f.Name] = uint64(i)
			}
		}
		return &Struct{Fields: fields, Tags: tags, FieldIndices: indices}

	case *ast.FuncType:
		params, _, isVariadic := c.collectFields(token.FUNC, t.Params, true)
		results, _, _ := c.collectFields(token.FUNC, t.Results, true)
		return &Func{Recv: nil, Params: params, Results: results, IsVariadic: isVariadic}

	case *ast.InterfaceType:
		methods, _, _ := c.collectFields(token.INTERFACE, t.Methods, cycleOk)
		methods.Sort()
		return &Interface{Methods: methods}

	case *ast.MapType:
		return &Map{Key: c.makeType(t.Key, true), Elt: c.makeType(t.Key, true)}

	case *ast.ChanType:
		return &Chan{Dir: t.Dir, Elt: c.makeType(t.Value, true)}
	}

	panic(fmt.Sprintf("unreachable (%T)", x))
}

// checkObj type checks an object.
func (c *checker) checkObj(obj *ast.Object, ref bool) {
	if obj.Type != nil {
		// object has already been type checked
		return
	}

	switch obj.Kind {
	case ast.Bad:
		// ignore

	case ast.Con:
		// TODO(gri) complete this

	case ast.Typ:
		typ := &Name{Obj: obj}
		obj.Type = typ // "mark" object so recursion terminates
		typ.Underlying = Underlying(c.makeType(obj.Decl.(*ast.TypeSpec).Type, ref))

		if methobjs := c.methods[obj]; methobjs != nil {
			for _, methobj := range methobjs {
				c.checkObj(methobj, ref)
			}
			methobjs.Sort()
			typ.Methods = methobjs
		}

	case ast.Var:
		// TODO(gri) complete this

	case ast.Fun:
		fndecl := obj.Decl.(*ast.FuncDecl)
		obj.Type = c.makeType(fndecl.Type, ref)
		fn := obj.Type.(*Func)
		if fndecl.Recv != nil {
			recvField := fndecl.Recv.List[0]
			if len(recvField.Names) > 0 {
				fn.Recv = recvField.Names[0].Obj
			} else {
				fn.Recv = ast.NewObj(ast.Var, "_")
				fn.Recv.Type = c.makeType(recvField.Type, ref)
			}
		}

	default:
		panic("unreachable")
	}
}

// Check typechecks a package.
// It augments the AST by assigning types to all ast.Objects and returns a map
// of types for all expression nodes in statements, and a scanner.ErrorList if
// there are errors.
//
func Check(fset *token.FileSet, pkg *ast.Package) (types map[ast.Expr]Type, err error) {
	var c checker
	c.fset = fset
	c.types = make(map[ast.Expr]Type)
	c.methods = make(map[*ast.Object]ObjList)

	for _, file := range pkg.Files {
		c.collectMethods(file)
	}

	for _, obj := range pkg.Scope.Objects {
		c.checkObj(obj, false)
	}

	c.errors.RemoveMultiples()
	return c.types, c.errors.Err()
}

// vim: set ft=go :
