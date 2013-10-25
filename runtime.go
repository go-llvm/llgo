// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"fmt"
	"go/build"
	"go/token"
	"path"

	"code.google.com/p/go.tools/go/types"

	"github.com/axw/gollvm/llvm"
)

type runtimeType struct {
	types.Type
	llvm llvm.Type
}

// runtimeInterface is a struct containing references to
// runtime types and intrinsic function declarations.
type runtimeInterface struct {
	// runtime types
	rtype,
	uncommonType,
	arrayType,
	chanType,
	funcType,
	method,
	imethod,
	interfaceType,
	mapType,
	ptrType,
	sliceType,
	structType runtimeType

	// intrinsics
	compareI2I,
	eqtyp,
	//fflush,
	llvm_trap,
	main,
	printfloat,
	malloc,
	memcpy,
	memequal,
	memset,
	panic_,
	recover_,
	rundefers,
	chancap,
	chanlen,
	maplen,
	makeslice,
	sliceappend,
	slicecopy,
	sliceslice,
	stringslice,
	strcat,
	strcmp,
	strnext,
	strrune,
	strtorunes,
	runestostr,
	streqalg,
	f32eqalg,
	f64eqalg,
	c64eqalg,
	c128eqalg *LLVMValue
}

func newRuntimeInterface(pkg *types.Package, module llvm.Module, tm *llvmTypeMap) (*runtimeInterface, error) {
	var ri runtimeInterface
	types := map[string]*runtimeType{
		"rtype":         &ri.rtype,
		"uncommonType":  &ri.uncommonType,
		"arrayType":     &ri.arrayType,
		"chanType":      &ri.chanType,
		"funcType":      &ri.funcType,
		"method":        &ri.method,
		"imethod":       &ri.imethod,
		"interfaceType": &ri.interfaceType,
		"mapType":       &ri.mapType,
		"ptrType":       &ri.ptrType,
		"sliceType":     &ri.sliceType,
		"structType":    &ri.structType,
	}
	for name, field := range types {
		obj := pkg.Scope().Lookup(name)
		if obj == nil {
			return nil, fmt.Errorf("no runtime type with name %s", name)
		}
		field.Type = obj.Type()
		field.llvm = tm.ToLLVM(field.Type)
	}

	intrinsics := map[string]**LLVMValue{
		"compareI2I": &ri.compareI2I,
		"eqtyp":      &ri.eqtyp,
		//"fflush": &ri.fflush,
		"llvm_trap":   &ri.llvm_trap,
		"main":        &ri.main,
		"printfloat":  &ri.printfloat,
		"malloc":      &ri.malloc,
		"memcpy":      &ri.memcpy,
		"memequal":    &ri.memequal,
		"memset":      &ri.memset,
		"panic_":      &ri.panic_,
		"recover_":    &ri.recover_,
		"rundefers":   &ri.rundefers,
		"chancap":     &ri.chancap,
		"chanlen":     &ri.chanlen,
		"maplen":      &ri.maplen,
		"makeslice":   &ri.makeslice,
		"sliceappend": &ri.sliceappend,
		"slicecopy":   &ri.slicecopy,
		"sliceslice":  &ri.sliceslice,
		"stringslice": &ri.stringslice,
		"strcat":      &ri.strcat,
		"strcmp":      &ri.strcmp,
		"strnext":     &ri.strnext,
		"strrune":     &ri.strrune,
		"strtorunes":  &ri.strtorunes,
		"runestostr":  &ri.runestostr,
		"streqalg":    &ri.streqalg,
		"f32eqalg":    &ri.f32eqalg,
		"f64eqalg":    &ri.f64eqalg,
		"c64eqalg":    &ri.c64eqalg,
		"c128eqalg":   &ri.c128eqalg,
	}
	for name, field := range intrinsics {
		obj := pkg.Scope().Lookup(name)
		if obj == nil {
			return nil, fmt.Errorf("no runtime function with name %s", name)
		}
		ftyp := obj.Type()
		llftyp := tm.ToLLVM(ftyp).StructElementTypes()[0].ElementType()
		llfn := llvm.AddFunction(module, "runtime."+name, llftyp)
		*field = &LLVMValue{value: llfn, typ: obj.Type()}
	}

	return &ri, nil
}

// parseRuntime parses the runtime package and type-checks its AST.
// This is used to generate runtime type structures.
func parseRuntime(buildctx *build.Context, checker *types.Config) (*types.Package, error) {
	buildpkg, err := buildctx.Import("github.com/axw/llgo/pkg/runtime", "", 0)
	if err != nil {
		return nil, err
	}
	filenames := make([]string, len(buildpkg.GoFiles))
	for i, f := range buildpkg.GoFiles {
		filenames[i] = path.Join(buildpkg.Dir, f)
	}
	fmt.Println(filenames)
	fset := token.NewFileSet()
	files, err := parseFiles(fset, filenames)
	if err != nil {
		return nil, err
	}
	pkg, err := checker.Check("runtime", fset, files, nil)
	if err != nil {
		return nil, err
	}
	return pkg, nil
}

func (c *compiler) createMalloc(size llvm.Value) llvm.Value {
	malloc := c.runtime.malloc.LLVMValue()
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
	memset := c.runtime.memset.LLVMValue()
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
