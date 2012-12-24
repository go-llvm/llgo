/*
Copyright (c) 2011, 2012 Andrew Wilkins <axwalk@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package llgo

import (
	"fmt"
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo/types"
	"go/ast"
	"go/token"
	"log"
	"strconv"
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
	types.TypeInfoConfig
	Compile(fset *token.FileSet, pkg *ast.Package, importpath string, exprtypes map[ast.Expr]types.Type) (*Module, error)
	Dispose()
}

type FunctionStack []*LLVMValue

func (s *FunctionStack) Push(v *LLVMValue) {
	*s = append(*s, v)
}

func (s *FunctionStack) Pop() *LLVMValue {
	f := (*s)[len(*s)-1]
	*s = (*s)[:len(*s)-1]
	return f
}

func (s *FunctionStack) Top() *LLVMValue {
	return (*s)[len(*s)-1]
}

type compiler struct {
	CompilerOptions

	builder        llvm.Builder
	module         *Module
	machine        llvm.TargetMachine
	target         llvm.TargetData
	functions      FunctionStack
	breakblocks    []llvm.BasicBlock
	continueblocks []llvm.BasicBlock
	initfuncs      []Value
	varinitfuncs   []Value
	pkg            *ast.Package
	importpath     string
	fileset        *token.FileSet
	filescope      *ast.Scope
	scope          *ast.Scope
	pkgmap         map[*ast.Object]string

	// forlabel, if non-nil, is a LabeledStmt immediately
	// preceding an unprocessed ForStmt, SwitchStmt or SelectStmt.
	// Upon processing the statement, the label data will be updated,
	// and forlabel set to nil.
	lastlabel *ast.Ident

	*FunctionCache
	llvmtypes *LLVMTypeMap
	types     *TypeMap
}

func (c *compiler) LookupObj(name string) *ast.Object {
	for scope := c.scope; scope != nil; scope = scope.Outer {
		obj := scope.Lookup(name)
		if obj != nil {
			return obj
		}
	}
	return nil
}

func (c *compiler) Resolve(obj *ast.Object) Value {
	if obj.Kind == ast.Pkg {
		return nil
	} else if obj.Kind == ast.Typ {
		return TypeValue{obj.Type.(types.Type)}
	}

	value, isvalue := (obj.Data).(Value)
	if isvalue {
		return value
	}

	switch obj.Kind {
	case ast.Con:
		if obj.Decl != nil {
			valspec := obj.Decl.(*ast.ValueSpec)
			c.VisitValueSpec(valspec, true)
			value = (obj.Data).(Value)
		} else if obj == types.Nil {
			return NilValue{c}
		} else {
			typ := obj.Type.(types.Type)
			value = ConstValue{(obj.Data.(types.Const)), c, typ}
			obj.Data = value
		}

	case ast.Fun:
		var funcdecl *ast.FuncDecl
		if obj.Decl != nil {
			funcdecl = obj.Decl.(*ast.FuncDecl)
		} else {
			funcdecl = &ast.FuncDecl{
				Name: &ast.Ident{Name: obj.Name, Obj: obj},
			}
		}
		value = c.VisitFuncProtoDecl(funcdecl)
		obj.Data = value

	case ast.Var:
		switch x := (obj.Decl).(type) {
		case *ast.ValueSpec:
			c.VisitValueSpec(x, false)
		case *ast.Field:
			// No-op. Fields will be yielded for function
			// arg/recv/ret. We update the .Data field of the
			// object when we enter the function definition.
			if obj.Data == nil {
				panic("expected obj.Data value")
			}
		}

		// If it's an external variable, we'll need to create a global
		// value reference here. It may be possible for multiple objects
		// to refer to the same variable.
		if obj.Data == nil {
			module := c.module.Module
			t := obj.Type.(types.Type)
			name := c.pkgmap[obj] + "." + obj.Name
			g := module.NamedGlobal(name)
			if g.IsNil() {
				g = llvm.AddGlobal(module, c.types.ToLLVM(t), name)
			}
			obj.Data = c.NewLLVMValue(g, &types.Pointer{Base: t}).makePointee()
		}

		value = (obj.Data).(Value)
	}

	return value
}

func (c *compiler) Lookup(name string) (Value, *ast.Object) {
	obj := c.LookupObj(name)
	if obj != nil {
		return c.Resolve(obj), obj
	}
	return nil, nil
}

///////////////////////////////////////////////////////////////////////////////

// createPackageMap maps package-level objects to the import path of the
// package they're defined in.
func createPackageMap(pkg *ast.Package, pkgid string) map[*ast.Object]string {
	pkgmap := make(map[*ast.Object]string)
	for _, obj := range pkg.Scope.Objects {
		pkgmap[obj] = pkgid
	}
	for pkgid, pkgobj := range pkg.Imports {
		scope := pkgobj.Data.(*ast.Scope)
		for _, obj := range scope.Objects {
			// Use the package ID as the symbol name prefix.
			pkgmap[obj] = pkgid
		}
	}
	return pkgmap
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
	compiler.llvmtypes = NewLLVMTypeMap(compiler.target)
	return compiler, nil
}

func (compiler *compiler) Dispose() {
	if compiler.machine.C != nil {
		compiler.machine.Dispose()
		compiler.machine.C = nil
	}
}

func (compiler *compiler) Compile(fset *token.FileSet,
	pkg *ast.Package, importpath string,
	exprTypes map[ast.Expr]types.Type) (m *Module, err error) {

	// FIXME I'd prefer if we didn't modify global state. Perhaps
	// we should always take a copy of types.Universe?
	defer func() {
		types.Universe.Lookup("true").Data = types.Const{true}
		types.Universe.Lookup("false").Data = types.Const{false}
	}()

	// FIXME create a compilation state, rather than storing in 'compiler'.
	compiler.fileset = fset
	compiler.pkg = pkg
	compiler.importpath = importpath
	compiler.initfuncs = nil
	compiler.varinitfuncs = nil

	// Create a Builder, for building LLVM instructions.
	compiler.builder = llvm.GlobalContext().NewBuilder()
	defer compiler.builder.Dispose()

	// Create a Module, which contains the LLVM bitcode. Dispose it on panic,
	// otherwise we'll set a finalizer at the end. The caller may invoke
	// Dispose manually, which will render the finalizer a no-op.
	modulename := pkg.Name
	compiler.module = &Module{llvm.NewModule(modulename), modulename, false}
	compiler.module.SetTarget(compiler.TargetTriple)
	compiler.module.SetDataLayout(compiler.target.String())
	defer func() {
		if e := recover(); e != nil {
			compiler.module.Dispose()
			panic(e)
			//err = e.(error)
		}
	}()

	// Create a mapping from objects back to packages, so we can create the
	// appropriate symbol names.
	compiler.pkgmap = createPackageMap(pkg, importpath)

	// Create a struct responsible for mapping static types to LLVM types,
	// and to runtime/dynamic type values.
	var resolver Resolver = compiler
	compiler.FunctionCache = NewFunctionCache(compiler)
	compiler.types = NewTypeMap(compiler.llvmtypes, compiler.module.Module, importpath, exprTypes, compiler.FunctionCache, resolver)

	// Compile each file in the package.
	for _, file := range pkg.Files {
		file.Scope.Outer = pkg.Scope
		compiler.filescope = file.Scope
		compiler.scope = file.Scope
		compiler.fixConstDecls(file)
		for _, decl := range file.Decls {
			compiler.VisitDecl(decl)
		}
	}

	// Define intrinsics for use by the runtime: malloc, free, memcpy, etc.
	// These could be defined in LLVM IR, and may be moved there later.
	if pkg.Name == "runtime" {
		compiler.defineRuntimeIntrinsics()
	}

	// Export runtime type information.
	if pkg.Name == "runtime" {
		compiler.exportBuiltinRuntimeTypes()
	}

	// Wrap "main.main" in a call to runtime.main.
	if pkg.Name == "main" {
		compiler.createMainFunction()
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
	var initfuncs [][]Value
	if compiler.varinitfuncs != nil {
		initfuncs = append(initfuncs, compiler.varinitfuncs)
	}
	if compiler.initfuncs != nil {
		initfuncs = append(initfuncs, compiler.initfuncs)
	}
	if initfuncs != nil {
		ctortype := llvm.PointerType(llvm.FunctionType(llvm.VoidType(), nil, false), 0)
		var ctors []llvm.Value
		var index int = 0
		for _, initfuncs := range initfuncs {
			for _, fn := range initfuncs {
				fnval := fn.LLVMValue()
				fnval.SetName("__llgo.ctor." + compiler.importpath + strconv.Itoa(index))
				ctors = append(ctors, fnval)
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

func (c *compiler) createMainFunction() {
	// runtime.main is called by main, with argc, argv,
	// and a pointer to main.main.
	runtimeMain := c.NamedFunction("runtime.main", "func f(int32, **byte, func()) int32")
	mainMain := c.NamedFunction("main.main", "func f()")
	main := c.NamedFunction("main", "func f(int32, **byte) int32")
	entry := llvm.AddBasicBlock(main, "entry")
	c.builder.SetInsertPointAtEnd(entry)
	args := []llvm.Value{main.Param(0), main.Param(1), mainMain}
	result := c.builder.CreateCall(runtimeMain, args, "")
	c.builder.CreateRet(result)
}

// vim: set ft=go :
