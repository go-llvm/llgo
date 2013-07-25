// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/exact"
	"code.google.com/p/go.tools/go/types"
	"fmt"
	"github.com/axw/gollvm/llvm"
	"go/ast"
	"go/token"
	"log"
	"strings"
)

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

// TODO get rid of this, change compiler to Compiler.
type Compiler interface {
	Compile(fset *token.FileSet, files []*ast.File, importpath string) (*Module, error)
	Dispose()
}

type compiler struct {
	CompilerOptions

	builder        *Builder
	module         *Module
	machine        llvm.TargetMachine
	target         llvm.TargetData
	functions      functionStack
	breakblocks    []llvm.BasicBlock
	continueblocks []llvm.BasicBlock
	initfuncs      []llvm.Value
	varinitfuncs   []llvm.Value
	pkg            *types.Package
	fileset        *token.FileSet

	typeinfo      types.Info
	objectdata    map[types.Object]*ObjectData
	methodsets    map[types.Type]*methodset
	exportedtypes []types.Type

	// lastlabel, if non-nil, is a LabeledStmt immediately
	// preceding an unprocessed ForStmt, SwitchStmt or SelectStmt.
	// Upon processing the statement, the label data will be updated,
	// and forlabel set to nil.
	lastlabel *ast.Ident

	*FunctionCache
	llvmtypes *LLVMTypeMap
	types     *TypeMap

	// runtimetypespkg is the type-checked runtime/types.go file,
	// which is used for evaluating the types of runtime functions.
	runtimetypespkg *types.Package

	// pnacl is set to true if the target triple was originally
	// specified as "pnacl". This is necessary, as the TargetTriple
	// field will have been updated to the true triple used to
	// compile PNaCl modules.
	pnacl bool
}

func (c *compiler) archinfo() (intsize, ptrsize int64) {
	ptrsize = int64(c.target.PointerSize())
	if ptrsize >= 8 {
		intsize = 8
	} else {
		intsize = 4
	}
	return
}

func (c *compiler) Resolve(ident *ast.Ident) Value {
	obj := c.typeinfo.Objects[ident]
	data := c.objectdata[obj]
	if data.Value != nil {
		return data.Value
	}

	var value *LLVMValue
	switch obj := obj.(type) {
	case *types.Func:
		value = c.makeFunc(ident, obj.Type().(*types.Signature))
	case *synthFunc:
		value = c.makeFunc(ident, obj.Type().(*types.Signature))

	case *types.Var:
		if data.Ident.Obj != nil {
			switch decl := data.Ident.Obj.Decl.(type) {
			case *ast.ValueSpec:
				c.VisitValueSpec(decl)
			case *ast.Field:
				// No-op. Fields will be yielded for function
				// arg/recv/ret. We update the .Data field of the
				// object when we enter the function definition.
				if data.Value == nil {
					panic("expected object value")
				}
			}
		}

		// If it's an external variable, we'll need to create a global
		// value reference here. It may be possible for multiple objects
		// to refer to the same variable.
		value = data.Value
		if value == nil {
			module := c.module.Module
			t := obj.Type()
			name := data.Package.Path() + "." + obj.Name()
			g := module.NamedGlobal(name)
			if g.IsNil() {
				g = llvm.AddGlobal(module, c.types.ToLLVM(t), name)
			}
			value = c.NewValue(g, types.NewPointer(t)).makePointee()
		}

	case *types.Const:
		value = c.NewConstValue(obj.Val(), obj.Type())

	default:
		panic(fmt.Sprintf("unreachable (%T)", obj))
	}

	data.Value = value
	return value
}

///////////////////////////////////////////////////////////////////////////////

type CompilerOptions struct {
	// TargetTriple is the LLVM triple for the target.
	TargetTriple string

	// Logger is a logger used for tracing compilation.
	Logger *log.Logger
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

func (compiler *compiler) Compile(fset *token.FileSet, files []*ast.File, importpath string) (m *Module, err error) {
	// FIXME create a compilation state, rather than storing in 'compiler'.
	compiler.fileset = fset
	compiler.initfuncs = nil
	compiler.varinitfuncs = nil

	// If no import path is specified, or the package's
	// name (not path) is "main", then set the import
	// path to be the same as the package's name.
	if importpath == "" || files[0].Name.String() == "main" {
		importpath = files[0].Name.String()
	}

	// Type-check, and store object data.
	compiler.typeinfo.Types = make(map[ast.Expr]types.Type)
	compiler.typeinfo.Values = make(map[ast.Expr]exact.Value)
	compiler.typeinfo.Objects = make(map[*ast.Ident]types.Object)
	compiler.typeinfo.Implicits = make(map[ast.Node]types.Object)
	compiler.objectdata = make(map[types.Object]*ObjectData)
	compiler.methodsets = make(map[types.Type]*methodset)
	compiler.exportedtypes = nil
	compiler.llvmtypes = NewLLVMTypeMap(compiler.target)
	pkg, err := compiler.typecheck(importpath, fset, files)
	if err != nil {
		return nil, err
	}
	compiler.pkg = pkg

	// Create a Module, which contains the LLVM bitcode. Dispose it on panic,
	// otherwise we'll set a finalizer at the end. The caller may invoke
	// Dispose manually, which will render the finalizer a no-op.
	modulename := importpath
	compiler.module = &Module{llvm.NewModule(modulename), modulename, false}
	compiler.module.SetTarget(compiler.TargetTriple)
	compiler.module.SetDataLayout(compiler.target.String())
	defer func() {
		if e := recover(); e != nil {
			compiler.module.Dispose()
			panic(e)
		}
	}()

	// Create a struct responsible for mapping static types to LLVM types,
	// and to runtime/dynamic type values.
	var resolver Resolver = compiler
	compiler.FunctionCache = NewFunctionCache(compiler)
	compiler.types = NewTypeMap(compiler.llvmtypes, compiler.module.Module, importpath, compiler.FunctionCache, resolver)

	// Create a Builder, for building LLVM instructions.
	compiler.builder = newBuilder(compiler.types)
	defer compiler.builder.Dispose()

	// Compile each file in the package.
	for _, file := range files {
		for _, decl := range file.Decls {
			compiler.VisitDecl(decl)
		}
	}

	// Export runtime type information.
	compiler.exportRuntimeTypes()

	// Wrap "main.main" in a call to runtime.main.
	if importpath == "main" {
		err = compiler.createMainFunction()
		if err != nil {
			return nil, err
		}
	}

	// Create global constructors. The initfuncs/varinitfuncs
	// slices are in the order of visitation; we generate the
	// list of constructors in the reverse order.
	//
	// The llgo linker will link modules in the order of
	// package dependency, i.e. if A requires B, then llgo-link
	// will link the modules in the order A, B. The "runtime"
	// package is always last.
	//
	// At program initialisation, the runtime initialisation
	// function (runtime.main) will invoke the constructors
	// in reverse order.
	var initfuncs [][]llvm.Value
	if compiler.varinitfuncs != nil {
		initfuncs = append(initfuncs, compiler.varinitfuncs)
	}
	if compiler.initfuncs != nil {
		initfuncs = append(initfuncs, compiler.initfuncs)
	}
	if initfuncs != nil {
		ctortype := llvm.PointerType(llvm.Int8Type(), 0)
		var ctors []llvm.Value
		var index int = 0
		for _, initfuncs := range initfuncs {
			for _, fnptr := range initfuncs {
				name := fmt.Sprintf("__llgo.ctor.%s.%d", importpath, index)
				fnptr.SetName(name)
				fnptr = llvm.ConstBitCast(fnptr, ctortype)
				ctors = append(ctors, fnptr)
				index++
			}
		}
		for i, n := 0, len(ctors); i < n/2; i++ {
			ctors[i], ctors[n-i-1] = ctors[n-i-1], ctors[i]
		}
		ctorsInit := llvm.ConstArray(ctortype, ctors)
		ctorsVar := llvm.AddGlobal(compiler.module.Module, ctorsInit.Type(), "runtime.ctors")
		ctorsVar.SetInitializer(ctorsInit)
		ctorsVar.SetLinkage(llvm.AppendingLinkage)
	}

	// Create debug metadata.
	//compiler.createMetadata()

	return compiler.module, nil
}

func (c *compiler) createMainFunction() error {
	// In a PNaCl program (plugin), there should not be a "main.main";
	// instead, we expect a "main.CreateModule" function.
	// See pkg/nacl/ppapi/ppapi.go for more details.
	mainMain := c.module.NamedFunction("main.main")
	if c.pnacl {
		// PNaCl's libppapi_stub.a implements "main", which simply
		// calls through to PpapiPluginMain. We define our own "main"
		// so that we can capture argc/argv.
		if !mainMain.IsNil() {
			return fmt.Errorf("Found main.main")
		}
		pluginMain := c.NamedFunction("PpapiPluginMain", "func() int32")

		// Synthesise a main which has no return value. We could cast
		// PpapiPluginMain, but this is potentially unsafe as its
		// calling convention is unspecified.
		ftyp := llvm.FunctionType(llvm.VoidType(), nil, false)
		mainMain = llvm.AddFunction(c.module.Module, "main.main", ftyp)
		entry := llvm.AddBasicBlock(mainMain, "entry")
		c.builder.SetInsertPointAtEnd(entry)
		c.builder.CreateCall(pluginMain, nil, "")
		c.builder.CreateRetVoid()
	} else {
		mainMain = c.module.NamedFunction("main.main")
	}

	if mainMain.IsNil() {
		return fmt.Errorf("Could not find main.main")
	}

	// runtime.main is called by main, with argc, argv, argp,
	// and a pointer to main.main, which must be a niladic
	// function with no result.
	runtimeMain := c.NamedFunction("runtime.main", "func(int32, **byte, **byte, *int8) int32")
	main := c.NamedFunction("main", "func(int32, **byte, **byte) int32")
	entry := llvm.AddBasicBlock(main, "entry")
	c.builder.SetInsertPointAtEnd(entry)
	mainMain = c.builder.CreateBitCast(mainMain, runtimeMain.Type().ElementType().ParamTypes()[3], "")
	args := []llvm.Value{main.Param(0), main.Param(1), main.Param(2), mainMain}
	result := c.builder.CreateCall(runtimeMain, args, "")
	c.builder.CreateRet(result)
	return nil
}

// vim: set ft=go :
