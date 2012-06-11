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

// decomposeRepeatConsts looks for each instance of a constant list, and
// for constants without a value, associates the preceding constant's
// value and type (if any).
func (c *checker) decomposeRepeatConsts(file *ast.File) {
	for _, decl := range file.Decls {
		gendecl, ok := decl.(*ast.GenDecl)
		if ok && gendecl.Tok == token.CONST {
			var predValueSpec *ast.ValueSpec
			for _, spec := range gendecl.Specs {
				valspec := spec.(*ast.ValueSpec)
				if len(valspec.Values) > 0 {
					predValueSpec = valspec
				} else {
					valspec.Type = predValueSpec.Type
					valspec.Values = predValueSpec.Values
				}
			}
		}
	}
}

// untypedPriority eturns an integer priority value that corresponds
// to the given type's position in the sequence:
//     integer, character, floating-point, complex.
func untypedPriority(t Type) int {
	switch t {
	case Int.Underlying:
		return 0
	case Rune.Underlying:
		return 1
	case Float64.Underlying:
		return 2
	case Complex128.Underlying:
		return 3
	}
	panic(fmt.Sprintf("unhandled (%T, %v)", t, t))
}

// checkExpr type checks an expression and returns its type.
func (c *checker) checkExpr(x ast.Expr, assignees []*ast.Ident) (typ Type) {
	if typ, ok := c.types[x]; ok {
		return typ
	}
	defer func() {
		if typ != nil {
			c.types[x] = typ
		}
	}()
	// multi-value assignment handled within CallExpr.
	if len(assignees) == 1 {
		defer func() {
			a := assignees[0]
			if a.Obj.Type == nil {
				a.Obj.Type = typ
			} else {
				// TODO convert rhs type to lhs.
			}
		}()
	}

	switch x := x.(type) {
	case *ast.Ident:
		// type name
		obj := x.Obj
		if obj == nil {
			// unresolved identifier (error has been reported before)
			return &Bad{Msg: "unresolved identifier"}
		}
		if obj.Kind != ast.Var && obj.Kind != ast.Con && obj.Kind != ast.Fun {
			msg := c.errorf(x.Pos(),
				"%s is neither function, variable nor constant",
				x.Name)
			return &Bad{Msg: msg}
		}
		c.checkObj(obj, false)
		return obj.Type.(Type)

	case *ast.BasicLit:
		switch x.Kind {
		case token.INT:
			return Int.Underlying
		case token.FLOAT:
			return Float64.Underlying
		case token.IMAG:
			return Complex128.Underlying
		case token.CHAR:
			return Rune.Underlying
		case token.STRING:
			return String.Underlying
		}

	case *ast.CompositeLit:
		// TODO do this properly.
		return c.makeType(x.Type, false)

	case *ast.BinaryExpr:
		xType := c.checkExpr(x.X, nil)
		yType := c.checkExpr(x.Y, nil)
		_, xUntyped := xType.(*Basic)
		_, yUntyped := yType.(*Basic)
		switch x.Op {
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			// TODO check the operands are comparable.
			// TODO check when to use untyped bool.
			return Bool
		case token.SHL, token.SHR:
			// TODO check right operand is unsigned integer, or untyped
			// constant convertible to unsigned integer.

			// If the left operand of a non-constant shift expression is an
			// untyped constant, the type of the constant is what it would
			// be if the shift expression were replaced by its left operand
			// alone; the type is int if it cannot be determined from thexi
			// context
			// TODO check if rhs is non-const.
			//if xUntyped && !isConst(x.Y) {
			//}
			return Int
		default:
			if xUntyped && yUntyped {
				// Except for shift operations, if the operands of a binary
				// operation are different kinds of untyped constants, the
				// operation and, for non-boolean operations, the result use
				// the kind that appears later in this list:
				//     integer, character, floating-point, complex.
				switch x := untypedPriority(xType) - untypedPriority(yType); {
				case x <= 0:
					return xType
				case x > 0:
					return yType
				}
			} else if xUntyped {
				// Convert x.X's type to x.Y's.
				c.types[x.X] = yType
				return yType
			} else if yUntyped {
				// Convert x.Y's type to x.X's.
				c.types[x.Y] = xType
				return xType
			}
			return xType
		}
		panic("unreachable")

	case *ast.UnaryExpr:
		optype := c.checkExpr(x.X, nil)
		for {
			u := Underlying(optype)
			if u == optype {
				break
			}
		}

		switch x.Op {
		case token.ADD, token.SUB: // +, -
			// TODO allow int, float (complex?)
			return optype
		case token.NOT: // !
			// TODO Make sure it's a bool.
			return optype
		case token.XOR: // ^
			// TODO allow int
			return optype
		case token.MUL: // *
			if ptr, ok := optype.(*Pointer); ok {
				return ptr.Base
			}
			msg := c.errorf(x.Pos(), "cannot dereference non-pointer")
			return &Bad{Msg: msg}
		case token.AND: // &
			// TODO check operand is addressable
			return &Pointer{Base: optype}
		case token.ARROW: // <-
			// TODO check channel direction
			if ch, ok := optype.(*Chan); ok {
				if len(assignees) > 0 {
					assignees[0].Obj.Type = ch.Elt
					if len(assignees) == 2 {
						assignees[1].Obj.Type = Bool
					}
				}
				return ch.Elt
			}
			msg := c.errorf(x.Pos(), "cannot receive from non-channel")
			return &Bad{Msg: msg}
		}
		panic("unreachable")

	case *ast.CallExpr:
		args := x.Args

		// check for unsafe functions.
		if x, ok := x.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := x.X.(*ast.Ident); ok && ident.Obj.Data == Unsafe.Data {
				switch x.Sel.Name {
				case "Offsetof":
					if len(args) > 0 {
						if _, ok := args[0].(*ast.SelectorExpr); !ok {
							// TODO format args
							return &Bad{Msg: fmt.Sprintf("invalid expression %s.%s", x.X, x.Sel)}
						}
						// TODO check arg is a struct field selector.
					}
					fallthrough
				case "Alignof":
					fallthrough
				case "Sizeof":
					if len(args) < 1 {
						return &Bad{Msg: fmt.Sprintf("missing argument for %s.%s", x.X, x.Sel)}
					} else if len(args) > 1 {
						return &Bad{Msg: fmt.Sprintf("extra arguments for %s.%s", x.X, x.Sel)}
					}
				default:
					return &Bad{Msg: fmt.Sprintf("undefined: %s.%s", x.X, x.Sel)}
				}
				return Uintptr
			}
		}

		// TODO check arg types
		fntype := c.checkExpr(x.Fun, nil).(*Func)
		for _, obj := range fntype.Results {
			c.checkObj(obj, false)
		}
		if len(assignees) > 1 {
			for i, obj := range fntype.Results {
				if assignees[i].Obj.Type == nil {
					assignees[i].Obj.Type = obj.Type
				} else {
					// TODO convert rhs type to lhs.
				}
			}
		}
		if len(fntype.Results) == 1 {
			return fntype.Results[0].Type.(Type)
		}
		return nil // nil or multi-value

	case *ast.SelectorExpr:
		if ident, ok := x.X.(*ast.Ident); ok {
			// qualified identifier
			if obj := ident.Obj; obj != nil && obj.Kind == ast.Pkg {
				pkgscope := obj.Data.(*ast.Scope)
				x.Sel.Obj = pkgscope.Lookup(x.Sel.Name)
				return c.checkExpr(x.Sel, nil)
			}
		}

		t := c.checkExpr(x.X, nil)
		panic(fmt.Sprintf("beep %T", t))
		switch t.(type) {
		case *Interface:
		}
		//if lhs, isstruct, 

		// TODO(gri) can this really happen (the parser should have excluded this)?
		//msg := c.errorf(t.Pos(), "expected qualified identifier")
		//return &Bad{Msg: msg}
	}

	panic(fmt.Sprintf("unreachable (%T)", x))
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
		valspec := obj.Decl.(*ast.ValueSpec)
		if valspec.Type != nil {
			obj.Type = c.makeType(valspec.Type, ref)
			for _, name := range valspec.Names {
				name.Obj.Type = obj.Type
			}
		}
		if valspec.Values != nil {
			for i, name := range valspec.Names {
				c.checkExpr(valspec.Values[i], []*ast.Ident{name})
			}
		}

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
		var names []*ast.Ident
		var values []ast.Expr
		var typexpr ast.Expr
		switch x := obj.Decl.(type) {
		case *ast.ValueSpec:
			names = x.Names
			values = x.Values
			typexpr = x.Type
		case *ast.Field:
			names = x.Names
			typexpr = x.Type
		default:
			panic("unimplemented")
		}
		if names != nil { // nil for anonymous field
			var typ Type
			if typexpr != nil {
				typ = c.makeType(typexpr, ref)
				for _, name := range names {
					if name.Obj != nil {
						name.Obj.Type = typ
					}
				}
			}
			if len(values) == 1 && len(names) > 1 {
				// multi-value assignment
				c.checkExpr(values[0], names)
			} else if len(values) == len(names) {
				for i, name := range names {
					c.checkExpr(values[i], []*ast.Ident{name})
				}
			}
		}

	case ast.Fun:
		fndecl := obj.Decl.(*ast.FuncDecl)
		obj.Type = c.makeType(fndecl.Type, ref)
		fn := obj.Type.(*Func)
		if fndecl.Recv != nil {
			recvField := fndecl.Recv.List[0]
			if len(recvField.Names) > 0 {
				fn.Recv = recvField.Names[0].Obj
				c.checkObj(fn.Recv, ref)
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
		c.decomposeRepeatConsts(file)
		c.collectMethods(file)
	}

	for _, obj := range pkg.Scope.Objects {
		c.checkObj(obj, false)
	}

	c.errors.RemoveMultiples()
	return c.types, c.errors.Err()
}

// vim: set ft=go :
