// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo/types"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path"
	"strings"
)

type FunctionCache struct {
	*compiler
	functions map[string]llvm.Value
}

func NewFunctionCache(c *compiler) *FunctionCache {
	return &FunctionCache{c, make(map[string]llvm.Value)}
}

func (c *FunctionCache) NamedFunction(name string, signature string) llvm.Value {
	f, _ := c.functions[name+":"+signature]
	if !f.IsNil() {
		return f
	}

	if strings.HasPrefix(name, c.module.Name+".") {
		obj := c.pkg.Scope.Lookup(name[len(c.module.Name)+1:])
		value := c.Resolve(obj)
		f = value.LLVMValue()
	} else {
		fset := token.NewFileSet()
		code := `package runtime;import("unsafe");` + signature + `{panic()}`
		file, err := parser.ParseFile(fset, "", code, 0)
		if err != nil {
			panic(err)
		}

		// Parse the runtime package, since we may need to refer to
		// its types. TODO cache the package.
		buildpkg, err := build.Import("github.com/axw/llgo/pkg/runtime", "", 0)
		if err != nil {
			panic(err)
		}
		runtimefiles := make([]string, len(buildpkg.GoFiles))
		for i, f := range buildpkg.GoFiles {
			runtimefiles[i] = path.Join(buildpkg.Dir, f)
		}

		files, err := parseFiles(fset, runtimefiles)
		if err != nil {
			panic(err)
		}
		files["<src>"] = file
		pkg, err := ast.NewPackage(fset, files, types.GcImport, types.Universe)
		if err != nil {
			panic(err)
		}

		_, err = types.Check("", fset, pkg)
		if err != nil {
			panic(err)
		}

		fdecl := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
		ftype := fdecl.Name.Obj.Type.(*types.Func)
		llvmfptrtype := c.types.ToLLVM(ftype)
		f = llvm.AddFunction(c.module.Module, name, llvmfptrtype.ElementType())
	}
	c.functions[name+":"+signature] = f
	return f
}
