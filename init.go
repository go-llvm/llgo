// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"go/build"

	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// createProginit creates the llgo.main.proginit function,
// which calls each of the package's init functions in
// the appropriate order.
func (c *compiler) createProginit(mainpkg *types.Package, buildctx *build.Context) error {
	fntyp := llvm.FunctionType(llvm.VoidType(), nil, false)
	fn := llvm.AddFunction(c.module.Module, "llgo.main.proginit", fntyp)
	entry := llvm.AddBasicBlock(fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)
	done := make(map[string]bool)
	c.packageInitFunction("runtime", done)
	for _, pkg := range mainpkg.Imports() {
		if err := c.packageInitFunctions(pkg.Path(), buildctx, done); err != nil {
			return err
		}
	}
	c.packageInitFunction("main", done)
	c.builder.CreateRetVoid()
	return nil
}

func (c *compiler) packageInitFunctions(path string, buildctx *build.Context, done map[string]bool) error {
	if path == "unsafe" || path == "C" {
		return nil
	}
	if _, ok := done[path]; ok {
		return nil
	}
	pkg, err := buildctx.Import(path, "", 0)
	if err != nil {
		return err
	}
	for _, path := range pkg.Imports {
		c.packageInitFunctions(path, buildctx, done)
	}
	c.packageInitFunction(path, done)
	return nil
}

func (c *compiler) packageInitFunction(path string, done map[string]bool) {
	name := path + ".init"
	fn := c.module.NamedFunction(name)
	if fn.IsNil() {
		fntyp := llvm.FunctionType(llvm.VoidType(), nil, false)
		fn = llvm.AddFunction(c.module.Module, name, fntyp)
	}
	c.builder.CreateCall(fn, nil, "")
	done[path] = true
}
