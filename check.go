// Copyright 2013 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"go/ast"
	"go/token"
	"go/types"
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
	intsize, ptrsize := c.archinfo()
	exprtypes := make(ExprTypeMap)
	objectdata := make(map[types.Object]*ObjectData)
	var namedtypes []*types.NamedType
	ctx := &types.Context{
		IntSize: intsize,
		PtrSize: ptrsize,
		Expr: func(x ast.Expr, typ types.Type, val interface{}) {
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
					namedtypes = append(namedtypes, typ)
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
	//for _, pkg := range pkg.Imports {
	//	assocObjectPackages(pkg, c.objectdata)
	//}

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
		//fmt.Println("Insert", obj, "from", pkg.Name)
		//objectdata[obj].Package = pkg
		objectdata[obj] = &ObjectData{Package: pkg}
	}
	for _, pkg := range pkg.Imports {
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
	if sig.IsVariadic {
		last := sig.Params[len(sig.Params)-1]
		last.Type = &types.Slice{Elt: last.Type}
	}

	// Record methods against the receiver.
	if sig.Recv != nil {
		methodfunc := v.objects[name].(*types.Func)
		recvtyp := derefType(sig.Recv.Type)
		named := recvtyp.(*types.NamedType)
		methods, exists := v.methodsets[named]
		if !exists {
			methods = new(methodset)
			v.methodsets[named] = methods
		}
		methods.Insert(methodfunc)
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
