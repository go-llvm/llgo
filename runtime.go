// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"fmt"
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
		// its types. Can't be cached, because type-checking can't
		// be done twice on the AST.
		buildpkg, err := build.Import("github.com/axw/llgo/pkg/runtime", "", 0)
		if err != nil {
			panic(err)
		}

		// All types visible to the compiler are in "types.go".
		runtimefiles := []string{path.Join(buildpkg.Dir, "types.go")}

		files, err := parseFiles(fset, runtimefiles)
		if err != nil {
			panic(err)
		}
		files["<src>"] = file

		pkg, err := ast.NewPackage(fset, files, types.GcImport, types.Universe)
		if err != nil {
			panic(err)
		}

		_, err = types.Check("", c.compiler, fset, pkg)
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

func parseFile(fset *token.FileSet, name string) (*ast.File, error) {
	return parser.ParseFile(fset, name, nil, parser.DeclarationErrors)
}

func parseFiles(fset *token.FileSet, filenames []string) (files map[string]*ast.File, err error) {
	files = make(map[string]*ast.File)
	for _, filename := range filenames {
		var file *ast.File
		file, err = parseFile(fset, filename)
		if err != nil {
			return
		} else if file != nil {
			if files[filename] != nil {
				err = fmt.Errorf("%q: duplicate file", filename)
				return
			}
			files[filename] = file
		}
	}
	return
}

// parseReflect parses the reflect package and type-checks its AST.
// This is used to generate runtime type structures.
func (c *compiler) parseReflect() (*ast.Package, error) {
	buildpkg, err := build.Import("reflect", "", 0)
	if err != nil {
		return nil, err
	}

	filenames := make([]string, len(buildpkg.GoFiles))
	for i, f := range buildpkg.GoFiles {
		filenames[i] = path.Join(buildpkg.Dir, f)
	}
	fset := token.NewFileSet()
	files, err := parseFiles(fset, filenames)
	if err != nil {
		return nil, err
	}

	pkg, err := ast.NewPackage(fset, files, types.GcImport, types.Universe)
	if err != nil {
		return nil, err
	}

	_, err = types.Check(buildpkg.Name, c, fset, pkg)
	if err != nil {
		return nil, err
	}
	return pkg, nil
}

func (c *compiler) createMalloc(size llvm.Value) llvm.Value {
	malloc := c.NamedFunction("runtime.malloc", "func f(uintptr) unsafe.Pointer")
	if size.Type().IntTypeWidth() > c.target.IntPtrType().IntTypeWidth() {
		size = c.builder.CreateTrunc(size, c.target.IntPtrType(), "")
	}
	return c.builder.CreateCall(malloc, []llvm.Value{size}, "")
}

func (c *compiler) createTypeMalloc(t llvm.Type) llvm.Value {
	ptr := c.createMalloc(llvm.SizeOf(t))
	return c.builder.CreateIntToPtr(ptr, llvm.PointerType(t, 0), "")
}
