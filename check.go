// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
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

func (c *compiler) typecheck(pkgpath string, fset *token.FileSet, files []*ast.File) (*types.Package, error) {
	config := &types.Config{
		Sizeof:    c.llvmtypes.Sizeof,
		Alignof:   c.llvmtypes.Alignof,
		Offsetsof: c.llvmtypes.Offsetsof,
	}

	var info types.Info
	info.Values = c.typeinfo.Values
	info.Types = c.typeinfo.Types
	info.Implicits = c.typeinfo.Implicits
	info.Objects = make(map[*ast.Ident]types.Object)
	pkg, err := config.Check(pkgpath, fset, files, &info)
	if err != nil {
		return nil, err
	}

	for id, obj := range info.Objects {
		c.typeinfo.Objects[id] = obj
		c.objectdata[obj] = &ObjectData{Ident: id, Package: pkg}
	}

	for _, pkg := range pkg.Imports() {
		assocObjectPackages(pkg, c.objectdata)
	}

	for object, data := range c.objectdata {
		if object, ok := object.(*types.TypeName); ok {
			// Add TypeNames to the LLVMTypeMap's TypeStringer.
			c.llvmtypes.pkgmap[object] = data.Package

			// Record exported types for generating runtime type information.
			// c.pkg is nil iff the package being checked is the package
			// being compiled.
			if c.pkg == nil && object.Pkg() == pkg && ast.IsExported(object.Name()) {
				c.exportedtypes = append(c.exportedtypes, object.Type())
			}
		}
	}

	return pkg, nil
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
