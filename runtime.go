// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
	"go/build"
	"go/token"
	"path"
)

type FunctionCache struct {
	*compiler
	functions map[string]llvm.Value
}

func NewFunctionCache(c *compiler) *FunctionCache {
	return &FunctionCache{c, make(map[string]llvm.Value)}
}

func (c *FunctionCache) RuntimeFunction(name string, signature string) llvm.Value {
	f, _ := c.functions[name]
	if !f.IsNil() {
		return f
	}

	if c.runtimetypespkg == nil {
		// Parse the runtime package, since we may need to refer to
		// its types.
		buildpkg, err := build.Import("github.com/axw/llgo/pkg/runtime", "", 0)
		if err != nil {
			panic(err)
		}

		// All types visible to the compiler are in "types.go".
		runtimefiles := []string{path.Join(buildpkg.Dir, "types.go")}

		fset := token.NewFileSet()
		files, err := parseFiles(fset, runtimefiles)
		if err != nil {
			panic(err)
		}
		c.runtimetypespkg, err = c.typechecker.Check("runtime", fset, files, nil)
		if err != nil {
			panic(err)
		}
	}

	pkg := c.runtimetypespkg
	scope := pkg.Scope().Child(0)
	ftype, _, err := types.Eval(signature+"{panic()}", pkg, scope)
	if err != nil {
		panic(err)
	}
	llvmfntyp := c.types.ToLLVM(ftype).StructElementTypes()[0].ElementType()
	f = llvm.AddFunction(c.module.Module, name, llvmfntyp)
	c.functions[name] = f
	return f
}

// parseReflect parses the reflect package and type-checks its AST.
// This is used to generate runtime type structures.
func (c *compiler) parseReflect() (*types.Package, error) {
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

	pkg, err := c.typechecker.Check("reflect", fset, files, nil)
	if err != nil {
		return nil, err
	}

	return pkg, nil
}

func (c *compiler) createMalloc(size llvm.Value) llvm.Value {
	malloc := c.RuntimeFunction("runtime.malloc", "func(uintptr) unsafe.Pointer")
	switch n := size.Type().IntTypeWidth() - c.target.IntPtrType().IntTypeWidth(); {
	case n < 0:
		size = c.builder.CreateZExt(size, c.target.IntPtrType(), "")
	case n > 0:
		size = c.builder.CreateTrunc(size, c.target.IntPtrType(), "")
	}
	return c.builder.CreateCall(malloc, []llvm.Value{size}, "")
}

func (c *compiler) createTypeMalloc(t llvm.Type) llvm.Value {
	ptr := c.createMalloc(llvm.SizeOf(t))
	return c.builder.CreateIntToPtr(ptr, llvm.PointerType(t, 0), "")
}

func (c *compiler) memsetZero(ptr llvm.Value, size llvm.Value) {
	memset := c.RuntimeFunction("runtime.memset", "func(dst unsafe.Pointer, fill byte, size uintptr)")
	switch n := size.Type().IntTypeWidth() - c.target.IntPtrType().IntTypeWidth(); {
	case n < 0:
		size = c.builder.CreateZExt(size, c.target.IntPtrType(), "")
	case n > 0:
		size = c.builder.CreateTrunc(size, c.target.IntPtrType(), "")
	}
	ptr = c.builder.CreatePtrToInt(ptr, c.target.IntPtrType(), "")
	fill := llvm.ConstNull(llvm.Int8Type())
	c.builder.CreateCall(memset, []llvm.Value{ptr, fill, size}, "")
}
