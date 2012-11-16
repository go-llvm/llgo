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
	"os"
	"runtime"
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

type Compiler interface {
	Compile(fset *token.FileSet, pkg *ast.Package, importpath string, exprtypes map[ast.Expr]types.Type) (*Module, error)
	SetTraceEnabled(bool)
	SetTargetArch(string)
	SetTargetOs(string)
	GetTargetTriple() string
}

type compiler struct {
	builder        llvm.Builder
	module         *Module
	targetArch     string
	targetOs       string
	target         llvm.TargetData
	functions      []Value
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
	*FunctionCache
	types  *TypeMap
	logger *log.Logger
}

func (c *compiler) LookupObj(name string) *ast.Object {
	// TODO check for qualified identifiers (x.y), and short-circuit the
	// lookup.
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

func NewCompiler() Compiler {
	compiler := new(compiler)
	compiler.SetTargetArch(runtime.GOARCH)
	compiler.SetTargetOs(runtime.GOOS)
	return compiler
}

func (c *compiler) SetTraceEnabled(enabled bool) {
	if enabled {
		c.logger = log.New(os.Stderr, "", 0)
	} else {
		c.logger = nil
	}
}

// SetTargetArch sets the target architecture, which must be either one of the
// architecture names recognised by the gc compiler, or an LLVM architecture
// name.
func (c *compiler) SetTargetArch(arch string) {
	switch arch {
	case "386":
		c.targetArch = "x86"
	case "amd64", "x86_64":
		c.targetArch = "x86-64"
	default:
		c.targetArch = arch
	}
}

// SetTargetOs sets the target OS, which must be either one of the OS names
// recognised by the gc compiler, or an LLVM OS name.
func (c *compiler) SetTargetOs(os string) {
	if os == "windows" {
		c.targetOs = "win32"
	} else {
		c.targetOs = os
	}
}

// Convert the architecture name to the string used in LLVM triples.
// See: llvm::Triple::getArchTypeName.
//
// TODO move this into the LLVM C API.
func getTripleArchName(llvmArch string) string {
	switch llvmArch {
	case "x86":
		return "i386"
	case "x86-64":
		return "x86_64"
	case "ppc32":
		return "powerpc"
	case "ppc64":
		return "powerpc64"
	}
	return llvmArch
}

func (compiler *compiler) GetTargetTriple() string {
	arch := getTripleArchName(compiler.targetArch)
	os := compiler.targetOs
	return fmt.Sprintf("%s-unknown-%s", arch, os)
}

func (compiler *compiler) Compile(fset *token.FileSet,
	pkg *ast.Package, importpath string,
	exprTypes map[ast.Expr]types.Type) (m *Module, err error) {
	// FIXME create a compilation state, rather than storing in 'compiler'.
	compiler.fileset = fset
	compiler.pkg = pkg
	compiler.importpath = importpath
	compiler.initfuncs = nil
	compiler.varinitfuncs = nil

	// Create a Builder, for building LLVM instructions.
	compiler.builder = llvm.GlobalContext().NewBuilder()
	defer compiler.builder.Dispose()

	// Create a TargetMachine from the OS & Arch.
	triple := compiler.GetTargetTriple()
	var machine llvm.TargetMachine
	for target := llvm.FirstTarget(); target.C != nil && machine.C == nil; target = target.NextTarget() {
		if target.Name() == compiler.targetArch {
			machine = target.CreateTargetMachine(triple, "", "",
				llvm.CodeGenLevelDefault,
				llvm.RelocDefault,
				llvm.CodeModelDefault)
			defer machine.Dispose()
		}
	}

	if machine.C == nil {
		err = fmt.Errorf("Invalid target triple: %s", triple)
		return
	}

	// Create a Module, which contains the LLVM bitcode. Dispose it on panic,
	// otherwise we'll set a finalizer at the end. The caller may invoke
	// Dispose manually, which will render the finalizer a no-op.
	modulename := pkg.Name
	compiler.target = machine.TargetData()
	compiler.module = &Module{llvm.NewModule(modulename), modulename, false}
	compiler.module.SetTarget(triple)
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
	llvmtypemap := NewLLVMTypeMap(compiler.module.Module, compiler.target)
	compiler.FunctionCache = NewFunctionCache(compiler)
	compiler.types = NewTypeMap(llvmtypemap, exprTypes, compiler.FunctionCache, compiler.pkgmap, resolver)

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

	// Create global constructors.
	//
	// XXX When imports are handled, we'll need to defer creating
	//     llvm.global_ctors until we create an executable. This is
	//     due to (a) imports having to be initialised before the
	//     importer, and (b) LLVM having no specified order of
	//     initialisation for ctors with the same priority.
	var initfuncs [][]Value
	if compiler.varinitfuncs != nil {
		initfuncs = append(initfuncs, compiler.varinitfuncs)
	}
	if compiler.initfuncs != nil {
		initfuncs = append(initfuncs, compiler.initfuncs)
	}
	if initfuncs != nil {
		elttypes := []llvm.Type{llvm.Int32Type(), llvm.PointerType(llvm.FunctionType(llvm.VoidType(), nil, false), 0)}
		ctortype := llvm.StructType(elttypes, false)
		var ctors []llvm.Value
		for priority, initfuncs := range initfuncs {
			priority := llvm.ConstInt(llvm.Int32Type(), uint64(priority), false)
			for _, fn := range initfuncs {
				struct_values := []llvm.Value{priority, fn.LLVMValue()}
				ctors = append(ctors, llvm.ConstStruct(struct_values, false))
			}
		}
		global_ctors_init := llvm.ConstArray(ctortype, ctors)
		global_ctors_var := llvm.AddGlobal(compiler.module.Module, global_ctors_init.Type(), "llvm.global_ctors")
		global_ctors_var.SetInitializer(global_ctors_init)
		global_ctors_var.SetLinkage(llvm.AppendingLinkage)
	}

	// Create debug metadata.
	//compiler.createMetadata()

	return compiler.module, nil
}

// vim: set ft=go :
