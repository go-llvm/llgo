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

	"code.google.com/p/go.tools/go/types"
	goimporter "code.google.com/p/go.tools/importer"
	"code.google.com/p/go.tools/ssa"
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
	Disposed bool
}

func (m Module) Dispose() {
	if !m.Disposed {
		m.Disposed = true
		m.Module.Dispose()
	}
}

type Compiler interface {
	Compile(filenames []string, importpath string) (*Module, error)
	Dispose()
}

type compiler struct {
	CompilerOptions

	builder, allocaBuilder llvm.Builder
	module  *Module
	machine llvm.TargetMachine
	target  llvm.TargetData
	fileset *token.FileSet

	typechecker *types.Config
	importer    *goimporter.Importer

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

	debug_context []llvm.DebugDescriptor
	debug_info    *llvm.DebugInfo
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
}

// Based on parseArch from LLVM's lib/Support/Triple.cpp.
// This is used to match the target machine type.
func parseArch(arch string) string {
	switch arch {
	case "i386", "i486", "i586", "i686", "i786", "i886", "i986":
		return "x86"
	case "amd64", "x86_64":
		return "x86-64"
	case "powerpc":
		return "ppc"
	case "powerpc64", "ppu":
		return "ppc64"
	case "mblaze":
		return "mblaze"
	case "arm", "xscale":
		return "arm"
	case "thumb":
		return "thumb"
	case "spu", "cellspu":
		return "cellspu"
	case "msp430":
		return "msp430"
	case "mips", "mipseb", "mipsallegrex":
		return "mips"
	case "mipsel", "mipsallegrexel":
		return "mipsel"
	case "mips64", "mips64eb":
		return "mips64"
	case "mipsel64":
		return "mipsel64"
	case "r600", "hexagon", "sparc", "sparcv9", "tce",
		"xcore", "nvptx", "nvptx64", "le32", "amdil":
		return arch
	}
	if strings.HasPrefix(arch, "armv") {
		return "arm"
	} else if strings.HasPrefix(arch, "thumbv") {
		return "thumb"
	}
	return "unknown"
}

func NewCompiler(opts CompilerOptions) (Compiler, error) {
	compiler := &compiler{CompilerOptions: opts}
	if strings.ToLower(compiler.TargetTriple) == "pnacl" {
		compiler.TargetTriple = PNaClTriple
		compiler.pnacl = true
	}

	// Triples are several fields separated by '-' characters.
	// The first field is the architecture. The architecture's
	// canonical form may include a '-' character, which would
	// have been translated to '_' for inclusion in a triple.
	triple := compiler.TargetTriple
	arch := triple[:strings.IndexRune(triple, '-')]
	arch = parseArch(arch)
	var machine llvm.TargetMachine
	for target := llvm.FirstTarget(); target.C != nil; target = target.NextTarget() {
		if arch == target.Name() {
			machine = target.CreateTargetMachine(triple, "", "",
				llvm.CodeGenLevelDefault,
				llvm.RelocDefault,
				llvm.CodeModelDefault)
			compiler.machine = machine
			break
		}
	}

	if machine.C == nil {
		return nil, fmt.Errorf("Invalid target triple: %s", triple)
	}
	compiler.target = machine.TargetData()
	return compiler, nil
}

func (compiler *compiler) Dispose() {
	if compiler.machine.C != nil {
		compiler.machine.Dispose()
		compiler.machine.C = nil
	}
}

func (c *compiler) logf(format string, v ...interface{}) {
	if c.Logger != nil {
		c.Logger.Printf(format, v...)
	}
}

func (compiler *compiler) Compile(filenames []string, importpath string) (m *Module, err error) {
	// FIXME create a compilation state, rather than storing in 'compiler'.
	compiler.llvmtypes = NewLLVMTypeMap(llvm.GlobalContext(), compiler.target)

	buildctx, err := llgobuild.ContextFromTriple(compiler.TargetTriple)
	if err != nil {
		return nil, err
	}
	impcfg := &goimporter.Config{
		TypeChecker: types.Config{
			Import: llgoimporter.NewImporter(buildctx).Import,
			Sizes:  compiler.llvmtypes,
		},
		Build: &buildctx.Context,
	}
	compiler.typechecker = &impcfg.TypeChecker
	compiler.importer = goimporter.New(impcfg)
	program := ssa.NewProgram(compiler.importer.Fset, 0)
	astFiles, err := parseFiles(compiler.importer.Fset, filenames)
	if err != nil {
		return nil, err
	}
	// If no import path is specified, or the package's
	// name (not path) is "main", then set the import
	// path to be the same as the package's name.
	if pkgname := astFiles[0].Name.String(); importpath == "" || pkgname == "main" {
		importpath = pkgname
	}
	mainPkginfo := compiler.importer.CreatePackage(importpath, astFiles...)
	if mainPkginfo.Err != nil {
		return nil, mainPkginfo.Err
	}
	// First call CreatePackages to resolve imports, and then CreatePackage
	// to obtain the main package. The latter simply returns the package
	// created by the former.
	if err := program.CreatePackages(compiler.importer); err != nil {
		return nil, err
	}
	mainpkg := program.CreatePackage(mainPkginfo)

	// Create a Module, which contains the LLVM bitcode. Dispose it on panic,
	// otherwise we'll set a finalizer at the end. The caller may invoke
	// Dispose manually, which will render the finalizer a no-op.
	modulename := importpath
	compiler.module = &Module{llvm.NewModule(modulename), modulename, false}
	compiler.module.SetTarget(compiler.TargetTriple)
	compiler.module.SetDataLayout(compiler.target.String())

	// Map runtime types and functions.
	runtimePkginfo := mainPkginfo
	runtimePkg := mainpkg
	if importpath != "runtime" {
		astFiles, err := parseRuntime(&buildctx.Context, compiler.importer.Fset)
		if err != nil {
			return nil, err
		}
		runtimePkginfo = compiler.importer.CreatePackage("runtime", astFiles...)
		if runtimePkginfo.Err != nil {
			return nil, err
		}
		runtimePkg = program.CreatePackage(runtimePkginfo)
	}

	// Create a new translation unit.
	unit := newUnit(compiler, mainpkg)

	// Create the runtime interface.
	compiler.runtime, err = newRuntimeInterface(
		runtimePkg.Object,
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

	mainpkg.Build()
	unit.translatePackage(mainpkg)
	compiler.processAnnotations(unit, mainPkginfo)
	if runtimePkginfo != mainPkginfo {
		compiler.processAnnotations(unit, runtimePkginfo)
	}

	compiler.types.finalize()

	/*
		compiler.debug_info = &llvm.DebugInfo{}
		// Compile each file in the package.
		for _, file := range files {
			if compiler.GenerateDebug {
				cu := &llvm.CompileUnitDescriptor{
					Language: llvm.DW_LANG_Go,
					Path:     llvm.FileDescriptor(fset.File(file.Pos()).Name()),
					Producer: LLGOProducer,
					Runtime:  LLGORuntimeVersion,
				}
				compiler.pushDebugContext(cu)
				compiler.pushDebugContext(&cu.Path)
			}
			for _, decl := range file.Decls {
				compiler.VisitDecl(decl)
			}
			if compiler.GenerateDebug {
				compiler.popDebugContext()
				cu := compiler.popDebugContext()
				if len(compiler.debug_context) > 0 {
					log.Panicln(compiler.debug_context)
				}
				compiler.module.AddNamedMetadataOperand(
					"llvm.dbg.cu",
					compiler.debug_info.MDNode(cu),
				)
			}
		}
	*/

	// Export runtime type information.
	var exportedTypes []types.Type
	for _, m := range mainpkg.Members {
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
		if err := llgoimporter.Export(buildctx, mainpkg.Object); err != nil {
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

	c.builder.SetCurrentDebugLocation(c.debug_info.MDNode(nil))
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
