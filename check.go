// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"fmt"
	"go/ast"
	"go/token"
)

// ObjectData stores information for a types.Object
type ObjectData struct {
	Ident *ast.Ident
	Value *LLVMValue
}

func (c *compiler) typecheck(pkgpath string, fset *token.FileSet, files []*ast.File) (*types.Package, error) {
	var errors string
	var imp = importer{compiler: c}
	config := &types.Config{
		Sizeof:    c.llvmtypes.Sizeof,
		Alignof:   c.llvmtypes.Alignof,
		Offsetsof: c.llvmtypes.Offsetsof,
		Error: func(err error) {
			if errors != "" {
				errors += "\n"
			}
			errors += err.Error()
		},
		Import: imp.Import,
	}

	var info types.Info
	objectdata := make(map[types.Object]*ObjectData)
	info.Values = c.typeinfo.Values
	info.Types = c.typeinfo.Types
	info.Selections = c.typeinfo.Selections
	info.Implicits = make(map[ast.Node]types.Object)
	info.Objects = make(map[*ast.Ident]types.Object)
	pkg, err := config.Check(pkgpath, fset, files, &info)
	if err != nil {
		return nil, fmt.Errorf("%s", errors)
	}

	for id, obj := range info.Objects {
		if obj == nil {
			continue
		}
		c.typeinfo.Objects[id] = obj
		data := objectdata[obj]
		if data == nil {
			objectdata[obj] = &ObjectData{Ident: id}
		} else if data.Ident.Obj == nil {
			data.Ident = id
		}
	}

	for node, obj := range info.Implicits {
		id := ast.NewIdent(obj.Name())
		c.typeinfo.Objects[id] = obj
		c.typeinfo.Implicits[node] = obj
		objectdata[obj] = &ObjectData{Ident: id}
	}

	for object, data := range objectdata {
		if object, ok := object.(*types.TypeName); ok {
			// Record exported types for generating runtime type information.
			// c.pkg is nil iff the package being checked is the package
			// being compiled.
			if c.pkg == nil && object.Pkg() == pkg && ast.IsExported(object.Name()) {
				c.exportedtypes = append(c.exportedtypes, object.Type())
			}
		}
		c.objectdata[object] = data
	}

	return pkg, nil
}
