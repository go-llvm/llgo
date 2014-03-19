// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"runtime"
	"strings"

	"github.com/axw/gollvm/llvm"
	llgobuild "github.com/axw/llgo/build"
	llgoimporter "github.com/axw/llgo/importer"

	"code.google.com/p/go.tools/go/gccgoimporter"
	"code.google.com/p/go.tools/go/loader"
	"code.google.com/p/go.tools/go/ssa"
	"code.google.com/p/go.tools/go/types"
)

func assert(cond bool) {
	if !cond {
		_, file, line, ok := runtime.Caller(1)
		if !ok {
			panic("assertion failed")
		}
		panic(fmt.Sprintf("assertion failed [%s:%d]", file, line))
	}
}

type Module struct {
	llvm.Module
	Name     string
	disposed bool
}

func (m *Module) Dispose() {
	if m.disposed {
		return
	}
	m.Module.Dispose()
	m.disposed = true
}

///////////////////////////////////////////////////////////////////////////////

type CompilerOptions struct {
	// TargetTriple is the LLVM triple for the target.
	TargetTriple string

	// GenerateDebug decides whether debug data is
	// generated in the output module.
	GenerateDebug bool

	// Logger is a logger used for tracing compilation.
	Logger *log.Logger

	// OrderedCompilation attempts to do some sorting to compile
	// functions in a deterministic order
	OrderedCompilation bool

	// GccgoPath is the path to the gccgo binary whose libgo we read import
	// data from
	GccgoPath string
}

type Compiler struct {
	opts       CompilerOptions
	dataLayout string
	pnacl      bool
}

func NewCompiler(opts CompilerOptions) (*Compiler, error) {
	compiler := &Compiler{opts: opts}
	if strings.ToLower(compiler.opts.TargetTriple) == "pnacl" {
		compiler.opts.TargetTriple = PNaClTriple
		compiler.pnacl = true
	}
	dataLayout, err := llvmDataLayout(compiler.opts.TargetTriple)
	if err != nil {
		return nil, err
	}
	compiler.dataLayout = dataLayout
	return compiler, nil
}

func (c *Compiler) Compile(filenames []string, importpath string) (m *Module, err error) {
	target := llvm.NewTargetData(c.dataLayout)
	compiler := &compiler{
		CompilerOptions: c.opts,
		dataLayout:      c.dataLayout,
		target:          target,
		pnacl:           c.pnacl,
		llvmtypes:       NewLLVMTypeMap(llvm.GlobalContext(), target),
	}
	return compiler.compile(filenames, importpath)
}

type compiler struct {
	CompilerOptions

	builder, allocaBuilder llvm.Builder
	module     *Module
	dataLayout string
	target     llvm.TargetData
	fileset    *token.FileSet

	runtime   *runtimeInterface
	llvmtypes *llvmTypeMap
	types     *TypeMap

	// runtimetypespkg is the type-checked runtime/types.go file,
	// which is used for evaluating the types of runtime functions.
	runtimetypespkg *types.Package

	// pnacl is set to true if the target triple was originally
	// specified as "pnacl". This is necessary, as the TargetTriple
	// field will have been updated to the true triple used to
	// compile PNaCl modules.
	pnacl bool

	debug debugInfo
}

func (c *compiler) logf(format string, v ...interface{}) {
	if c.Logger != nil {
		c.Logger.Printf(format, v...)
	}
}

func (compiler *compiler) compile(filenames []string, importpath string) (m *Module, err error) {
	buildctx, err := llgobuild.ContextFromTriple(compiler.TargetTriple)
	if err != nil {
		return nil, err
	}
	var inst gccgoimporter.GccgoInstallation
	err = inst.InitFromDriver(compiler.GccgoPath)
	if err != nil {
		return nil, err
	}
	impcfg := &loader.Config{
		Fset: token.NewFileSet(),
		TypeChecker: types.Config{
			Import: inst.GetImporter(nil),
			Sizes:  compiler.llvmtypes,
		},
		Build: &buildctx.Context,
	}
	// Must use parseFiles, so we retain comments;
	// this is important for annotation processing.
	astFiles, err := parseFiles(impcfg.Fset, filenames)
	if err != nil {
		return nil, err
	}
	// If no import path is specified, or the package's
	// name (not path) is "main", then set the import
	// path to be the same as the package's name.
	if pkgname := astFiles[0].Name.String(); importpath == "" || pkgname == "main" {
		importpath = pkgname
	}
	impcfg.CreateFromFiles(importpath, astFiles...)
	// Create a "runtime" package too, so we can reference
	// its types and functions in the compiler and generated
	// code.
	if importpath != "runtime" {
		astFiles, err := parseRuntime(&buildctx.Context, impcfg.Fset)
		if err != nil {
			return nil, err
		}
		impcfg.CreateFromFiles("runtime", astFiles...)
	}
	iprog, err := impcfg.Load()
	if err != nil {
		return nil, err
	}
	program := ssa.Create(iprog, 0)
	var mainPkginfo, runtimePkginfo *loader.PackageInfo
	if pkgs := iprog.InitialPackages(); len(pkgs) == 1 {
		mainPkginfo, runtimePkginfo = pkgs[0], pkgs[0]
	} else {
		mainPkginfo, runtimePkginfo = pkgs[0], pkgs[1]
	}
	mainPkg := program.CreatePackage(mainPkginfo)

	// Create a Module, which contains the LLVM bitcode.
	modulename := importpath
	compiler.module = &Module{Module: llvm.NewModule(modulename), Name: modulename}
	compiler.module.SetTarget(compiler.TargetTriple)
	compiler.module.SetDataLayout(compiler.dataLayout)

	// Create a new translation unit.
	unit := newUnit(compiler, mainPkg)

	// Create the runtime interface.
	compiler.runtime, err = newRuntimeInterface(
		runtimePkginfo.Pkg,
		compiler.module.Module,
		compiler.llvmtypes,
		FuncResolver(unit),
	)
	if err != nil {
		return nil, err
	}

	var mc manglerContext
	mc.init(program)

	// Create a struct responsible for mapping static types to LLVM types,
	// and to runtime/dynamic type values.
	compiler.types = NewTypeMap(
		importpath,
		compiler.llvmtypes,
		compiler.module.Module,
		compiler.runtime,
		MethodResolver(unit),
		&mc,
	)

	// Create a Builder, for building LLVM instructions.
	compiler.builder = llvm.GlobalContext().NewBuilder()
	defer compiler.builder.Dispose()

	compiler.allocaBuilder = llvm.GlobalContext().NewBuilder()
	defer compiler.allocaBuilder.Dispose()

	// Initialise debugging.
	compiler.debug.module = compiler.module.Module
	compiler.debug.Fset = impcfg.Fset
	compiler.debug.Sizes = compiler.llvmtypes

	mainPkg.Build()
	unit.translatePackage(mainPkg)
	compiler.processAnnotations(unit, mainPkginfo)
	if runtimePkginfo != mainPkginfo {
		compiler.processAnnotations(unit, runtimePkginfo)
	}

	compiler.types.finalize()

	// Finalise debugging.
	for _, cu := range compiler.debug.cu {
		compiler.module.AddNamedMetadataOperand(
			"llvm.dbg.cu",
			compiler.debug.MDNode(cu),
		)
	}

	// Export runtime type information.
	var exportedTypes []types.Type
	for _, m := range mainPkg.Members {
		if t, ok := m.(*ssa.Type); ok && ast.IsExported(t.Name()) {
			exportedTypes = append(exportedTypes, t.Type())
		}
	}
	compiler.exportRuntimeTypes(exportedTypes, importpath == "runtime")

	/*
	if importpath == "main" {
		// Wrap "main.main" in a call to runtime.main.
		if err = compiler.createMainFunction(); err != nil {
			return nil, fmt.Errorf("failed to create main.main: %v", err)
		}
	} else {
	*/
		if err := llgoimporter.Export(buildctx, mainPkg.Object); err != nil {
			return nil, fmt.Errorf("failed to export package data: %v", err)
		}
	/*
	}
	*/

	return compiler.module, nil
}

func (c *compiler) createMainFunction() error {
	// In a PNaCl program (plugin), there should not be a "main.main";
	// instead, we expect a "main.CreateModule" function.
	// See pkg/nacl/ppapi/ppapi.go for more details.
	mainMain := c.module.NamedFunction("main.main")
	/*
		if c.pnacl {
			// PNaCl's libppapi_stub.a implements "main", which simply
			// calls through to PpapiPluginMain. We define our own "main"
			// so that we can capture argc/argv.
			if !mainMain.IsNil() {
				return fmt.Errorf("Found main.main")
			}
			pluginMain := c.RuntimeFunction("PpapiPluginMain", "func() int32")

			// Synthesise a main which has no return value. We could cast
			// PpapiPluginMain, but this is potentially unsafe as its
			// calling convention is unspecified.
			ftyp := llvm.FunctionType(llvm.VoidType(), nil, false)
			mainMain = llvm.AddFunction(c.module.Module, "main.main", ftyp)
			entry := llvm.AddBasicBlock(mainMain, "entry")
			c.builder.SetInsertPointAtEnd(entry)
			c.builder.CreateCall(pluginMain, nil, "")
			c.builder.CreateRetVoid()
		} else */{
		mainMain = c.module.NamedFunction("main.main")
	}

	if mainMain.IsNil() {
		return fmt.Errorf("Could not find main.main")
	}

	// runtime.main is called by main, with argc, argv, argp,
	// and a pointer to main.main, which must be a niladic
	// function with no result.
	runtimeMain := c.runtime.main.LLVMValue()

	ptrptr := llvm.PointerType(llvm.PointerType(llvm.Int8Type(), 0), 0)
	ftyp := llvm.FunctionType(llvm.Int32Type(), []llvm.Type{llvm.Int32Type(), ptrptr, ptrptr}, true)
	main := llvm.AddFunction(c.module.Module, "main", ftyp)

	c.builder.SetCurrentDebugLocation(c.debug.MDNode(nil))
	entry := llvm.AddBasicBlock(main, "entry")
	c.builder.SetInsertPointAtEnd(entry)
	runtimeMainParamTypes := runtimeMain.Type().ElementType().ParamTypes()
	args := []llvm.Value{
		main.Param(0), // argc
		main.Param(1), // argv
		main.Param(2), // argp
		c.builder.CreateBitCast(mainMain, runtimeMainParamTypes[3], ""),
	}
	result := c.builder.CreateCall(runtimeMain, args, "")
	c.builder.CreateRet(result)
	return nil
}

// vim: set ft=go :
