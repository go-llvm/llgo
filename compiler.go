// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"sort"
	"strings"

	llgobuild "github.com/go-llvm/llgo/build"
	llgoimporter "github.com/go-llvm/llgo/importer"
	"github.com/go-llvm/llvm"

	"code.google.com/p/go.tools/go/gccgoimporter"
	"code.google.com/p/go.tools/go/loader"
	"code.google.com/p/go.tools/go/ssa"
	"code.google.com/p/go.tools/go/types"
)

func addCommonFunctionAttrs(fn llvm.Value) {
	fn.AddTargetDependentFunctionAttr("disable-tail-calls", "true")
	fn.AddTargetDependentFunctionAttr("split-stack", "")
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
	iprog, err := impcfg.Load()
	if err != nil {
		return nil, err
	}
	program := ssa.Create(iprog, ssa.GccgoImport)
	mainPkginfo := iprog.InitialPackages()[0]
	mainPkg := program.CreatePackage(mainPkginfo)

	// Create a Module, which contains the LLVM bitcode.
	modulename := importpath
	compiler.module = &Module{Module: llvm.NewModule(modulename), Name: modulename}
	compiler.module.SetTarget(compiler.TargetTriple)
	compiler.module.SetDataLayout(compiler.dataLayout)

	// Create a new translation unit.
	unit := newUnit(compiler, mainPkg)

	// Create the runtime interface.
	compiler.runtime, err = newRuntimeInterface(compiler.module.Module, compiler.llvmtypes)
	if err != nil {
		return nil, err
	}

	// Create a struct responsible for mapping static types to LLVM types,
	// and to runtime/dynamic type values.
	compiler.types = NewTypeMap(
		mainPkg,
		compiler.llvmtypes,
		compiler.module.Module,
		compiler.runtime,
		MethodResolver(unit),
	)

	// Initialise debugging.
	compiler.debug.module = compiler.module.Module
	compiler.debug.Fset = impcfg.Fset
	compiler.debug.Sizes = compiler.llvmtypes

	mainPkg.Build()
	unit.translatePackage(mainPkg)
	compiler.processAnnotations(unit, mainPkginfo)

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

	if importpath == "main" {
		if err = compiler.createInitMainFunction(mainPkg); err != nil {
			return nil, fmt.Errorf("failed to create __go_init_main: %v", err)
		}
	} else {
		if err := llgoimporter.Export(buildctx, mainPkg.Object); err != nil {
			return nil, fmt.Errorf("failed to export package data: %v", err)
		}
	}

	return compiler.module, nil
}

type ByPriority []types.PackageInit

func (a ByPriority) Len() int           { return len(a) }
func (a ByPriority) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByPriority) Less(i, j int) bool { return a[i].Priority < a[j].Priority }

func (c *compiler) createInitMainFunction(mainPkg *ssa.Package) error {
	var inits []types.PackageInit
	for _, imp := range mainPkg.Object.Imports() {
		inits = append(inits, imp.Inits()...)
	}
	sort.Sort(ByPriority(inits))

	ftyp := llvm.FunctionType(llvm.VoidType(), nil, false)
	initMain := llvm.AddFunction(c.module.Module, "__go_init_main", ftyp)
	addCommonFunctionAttrs(initMain)
	entry := llvm.AddBasicBlock(initMain, "entry")

	builder := llvm.GlobalContext().NewBuilder()
	defer builder.Dispose()
	builder.SetInsertPointAtEnd(entry)

	seenInit := make(map[string]bool)

	for _, init := range inits {
		if seenInit[init.Function] {
			continue
		}
		seenInit[init.Function] = true

		initfn := llvm.AddFunction(c.module.Module, init.Function, ftyp)
		builder.CreateCall(initfn, nil, "")
	}

	maininitfn := c.module.Module.NamedFunction("main..import")
	if maininitfn.C != nil {
		builder.CreateCall(maininitfn, nil, "")
	}

	builder.CreateRetVoid()
	return nil
}

// vim: set ft=go :
