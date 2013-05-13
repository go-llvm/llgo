// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.exp/go/exact"
	"code.google.com/p/go.exp/go/types"
	"go/ast"
	"go/token"
	"sort"
)

// ObjectData stores information for a types.Object
type ObjectData struct {
	Ident   *ast.Ident
	Package *types.Package
	Value   *LLVMValue
}

type methodList []*types.Method

func (l methodList) Len() int           { return len(l) }
func (l methodList) Less(i, j int) bool { return l[i].Name < l[j].Name }
func (l methodList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }

func (c *compiler) typecheck(fset *token.FileSet, files []*ast.File) (*types.Package, ExprTypeMap, error) {
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

			switch obj := obj.(type) {
			case *types.TypeName:
				// Sort methods by name.
				switch typ := obj.Type.(type) {
				case *types.NamedType:
					sort.Sort(methodList(typ.Methods))
				case *types.Interface:
					sort.Sort(methodList(typ.Methods))
				}
			}
		},
	}
	pkg, err := ctx.Check(fset, files)
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
	for importpath, pkg := range pkg.Imports {
		// Methods of types in imported packages won't be
		// seen by ctx.Ident, so we need to handle them here.
		for _, obj := range pkg.Scope.Entries {
			switch obj := obj.(type) {
			case *types.Func:
				if sig, ok := obj.Type.(*types.Signature); ok {
					fixVariadicSignature(sig)
				}
			case *types.TypeName:
				if typ, ok := obj.Type.(*types.NamedType); ok {
					for _, m := range typ.Methods {
						fixVariadicSignature(m.Type)
					}
				}
			}
		}

		// Associate objects from imported packages
		// with the corresponding *types.Package.
		assocObjectPackages(pkg, c.objectdata)
		pkg.Path = importpath
	}

	// Add TypeNames to the LLVMTypeMap's TypeStringer.
	for object, data := range c.objectdata {
		if object, ok := object.(*types.TypeName); ok {
			c.llvmtypes.pkgmap[object] = data.Package
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
	for _, obj := range pkg.Scope.Entries {
		if data, ok := objectdata[obj]; ok {
			data.Package = pkg
		} else {
			objectdata[obj] = &ObjectData{Package: pkg}
		}
	}
	for importpath, pkg := range pkg.Imports {
		assocObjectPackages(pkg, objectdata)
		pkg.Path = importpath
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
	var name *ast.Ident

	switch node := node.(type) {
	case *ast.FuncDecl:
		sig = v.objects[node.Name].GetType().(*types.Signature)
		astfunc = node.Type
		noderecv = node.Recv
		name = node.Name
	case *ast.FuncLit:
		sig = v.exprtypes[node].Type.(*types.Signature)
		astfunc = node.Type
	default:
		return v
	}

	// If we have a variadic function, change its last
	// parameter's type to a slice of its recorded type.
	fixVariadicSignature(sig)

	// Record Method-to-Func mapping.
	if sig.Recv != nil {
		methodfunc := v.objects[name].(*types.Func)
		recvtyp := derefType(sig.Recv.Type)
		named := recvtyp.(*types.NamedType)
		for _, m := range named.Methods {
			if m.Name == name.Name {
				v.methodfuncs[m] = methodfunc
				break
			}
		}
	}

	// go/types creates a separate types.Var for
	// internal and external usage. We need to
	// associate them at the object data level.
	paramIdents := fieldlistIdents(astfunc.Params)
	resultIdents := fieldlistIdents(astfunc.Results)
	if sig.Recv != nil {
		id := fieldlistIdents(noderecv)[0]
		if obj, ok := v.objects[id]; ok {
			v.objectdata[sig.Recv] = v.objectdata[obj]
		}
	}
	for i, id := range paramIdents {
		if obj, ok := v.objects[id]; ok {
			v.objectdata[sig.Params[i]] = v.objectdata[obj]
		}
	}
	for i, id := range resultIdents {
		if obj, ok := v.objects[id]; ok {
			v.objectdata[sig.Results[i]] = v.objectdata[obj]
		}
	}
	return v
}

// fixVariadicSignature will change the last parameter type
// of a variadic function signature such that it is a slice
// of the type reported by go/types.
func fixVariadicSignature(sig *types.Signature) {
	if sig.IsVariadic {
		last := sig.Params[len(sig.Params)-1]
		last.Type = &types.Slice{Elt: last.Type}
	}
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
