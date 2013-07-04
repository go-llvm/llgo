// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/exact"
	"code.google.com/p/go.tools/go/types"
	"go/ast"
	"go/token"
)

// ObjectData stores information for a types.Object
type ObjectData struct {
	Ident   *ast.Ident
	Package *types.Package
	Value   *LLVMValue
}

func (c *compiler) typecheck(pkgpath string, fset *token.FileSet, files []*ast.File) (*types.Package, ExprTypeMap, error) {
	exprtypes := make(ExprTypeMap)
	objectdata := make(map[types.Object]*ObjectData)
	ctx := &types.Context{
		Sizeof:    c.llvmtypes.Sizeof,
		Alignof:   c.llvmtypes.Alignof,
		Offsetsof: c.llvmtypes.Offsetsof,
		Expr: func(x ast.Expr, typ types.Type, val exact.Value) {
			exprtypes[x] = ExprTypeInfo{Type: typ, Value: val}
		},
		Ident: func(id *ast.Ident, obj types.Object) {
			c.objects[id] = obj
			data := objectdata[obj]
			if data == nil {
				data = &ObjectData{Ident: id}
				objectdata[obj] = data
			}
		},
		ImplicitObj: func(node ast.Node, obj types.Object) {
			c.implicitobjects[node] = obj
		},
	}
	pkg, err := ctx.Check(pkgpath, fset, files...)
	if err != nil {
		return nil, nil, err
	}

	// Associate objects with the packages in which they were declared.
	//
	// Objects inside functions and so forth can't be got at directly,
	// so we'll initially set all objects' package to be the main package,
	// and then process the imported packages, as they'll only have
	// package-level objects of interest.
	for obj, data := range objectdata {
		data.Package = pkg
		c.objectdata[obj] = data
	}
	for _, pkg := range pkg.Imports() {
		// Associate objects from imported packages
		// with the corresponding *types.Package.
		assocObjectPackages(pkg, c.objectdata)
	}

	for object, data := range c.objectdata {
		if object, ok := object.(*types.TypeName); ok {
			// Add TypeNames to the LLVMTypeMap's TypeStringer.
			c.llvmtypes.pkgmap[object] = data.Package

			// Record exported types for generating runtime type information.
			if ast.IsExported(object.Name()) {
				c.exportedtypes = append(c.exportedtypes, object.Type())
			}
		}
	}

	// Preprocess the types/objects recorded by go/types
	// to make them suitable for consumption by llgo.
	v := funcTypeVisitor{c, exprtypes}
	for _, file := range files {
		ast.Walk(v, file)
	}

	return pkg, exprtypes, nil
}

func assocObjectPackages(pkg *types.Package, objectdata map[types.Object]*ObjectData) {
	for i := 0; i < pkg.Scope().NumEntries(); i++ {
		obj := pkg.Scope().At(i)
		if data, ok := objectdata[obj]; ok {
			data.Package = pkg
		} else {
			objectdata[obj] = &ObjectData{Package: pkg}
		}
	}
	for _, pkg := range pkg.Imports() {
		assocObjectPackages(pkg, objectdata)
	}
}

type funcTypeVisitor struct {
	*compiler
	exprtypes ExprTypeMap
}

func (v funcTypeVisitor) Visit(node ast.Node) ast.Visitor {
	var sig *types.Signature
	var noderecv *ast.FieldList
	var astfunc *ast.FuncType

	switch node := node.(type) {
	case *ast.FuncDecl:
		sig = v.objects[node.Name].Type().(*types.Signature)
		astfunc = node.Type
		noderecv = node.Recv
	case *ast.FuncLit:
		sig = v.exprtypes[node].Type.(*types.Signature)
		astfunc = node.Type
	default:
		return v
	}

	// go/types creates a separate types.Var for
	// internal and external usage. We need to
	// associate them at the object data level.
	paramIdents := fieldlistIdents(astfunc.Params)
	resultIdents := fieldlistIdents(astfunc.Results)
	if recv := sig.Recv(); recv != nil {
		id := fieldlistIdents(noderecv)[0]
		if obj, ok := v.objects[id]; ok {
			v.objectdata[recv] = v.objectdata[obj]
		}
	}
	for i, id := range paramIdents {
		if obj, ok := v.objects[id]; ok {
			v.objectdata[sig.Params().At(i)] = v.objectdata[obj]
		}
	}
	for i, id := range resultIdents {
		if obj, ok := v.objects[id]; ok {
			v.objectdata[sig.Results().At(i)] = v.objectdata[obj]
		}
	}
	return v
}

func fieldlistIdents(l *ast.FieldList) (idents []*ast.Ident) {
	if l != nil {
		for _, field := range l.List {
			if field.Names == nil {
				idents = append(idents, nil)
			}
			for _, name := range field.Names {
				idents = append(idents, name)
			}
		}
	}
	return
}
