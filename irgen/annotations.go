// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package irgen

import (
	"go/ast"
	"go/token"
	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types"
	"llvm.org/llvm/bindings/go/llvm"
)

// processAnnotations takes an *ssa.Package and a
// *importer.PackageInfo, and processes all of the
// llgo source annotations attached to each top-level
// function and global variable.
func (c *compiler) processAnnotations(u *unit, pkginfo *loader.PackageInfo) {
	members := make(map[types.Object]llvm.Value, len(u.globals))
	for k, v := range u.globals {
		members[k.(ssa.Member).Object()] = v
	}
	applyAttributes := func(attrs []Attribute, idents ...*ast.Ident) {
		if len(attrs) == 0 {
			return
		}
		for _, ident := range idents {
			if v := members[pkginfo.ObjectOf(ident)]; !v.IsNil() {
				for _, attr := range attrs {
					attr.Apply(v)
				}
			}
		}
	}
	for _, f := range pkginfo.Files {
		for _, decl := range f.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				attrs := parseAttributes(decl.Doc)
				applyAttributes(attrs, decl.Name)
			case *ast.GenDecl:
				if decl.Tok != token.VAR {
					continue
				}
				for _, spec := range decl.Specs {
					varspec := spec.(*ast.ValueSpec)
					attrs := parseAttributes(decl.Doc)
					applyAttributes(attrs, varspec.Names...)
				}
			}
		}
	}
}
