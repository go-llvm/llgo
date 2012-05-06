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
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo/types"
	"go/ast"
	"go/token"
	"log"
	"os"
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
	Compile(*token.FileSet, *ast.Package) (*Module, error)
	SetTraceEnabled(bool)
}

type compiler struct {
	builder   llvm.Builder
	module    *Module
	target    llvm.TargetData
	functions []Value
	initfuncs []Value
	pkg       *ast.Package
	fileset   *token.FileSet
	filescope *ast.Scope
	scope     *ast.Scope
	pkgmap    map[*ast.Object]string
	types     *TypeMap
	logger    *log.Logger
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
		} else {
			var typ *types.Basic
			switch x := obj.Type.(type) {
			case *types.Basic:
				typ = x
			case *types.Name:
				typ = x.Underlying.(*types.Basic)
			}
			value = ConstValue{*(obj.Data.(*types.Const)), c, typ}
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
		}

		// If it's an external variable, we'll need to create a global
		// value reference here.
		if obj.Data == nil {
			module := c.module.Module
			t := obj.Type.(types.Type)
			name := c.pkgmap[obj] + "." + obj.Name
			g := llvm.AddGlobal(module, c.types.ToLLVM(t), name)
			g.SetLinkage(llvm.AvailableExternallyLinkage)
			obj.Data = c.NewLLVMValue(g, t)
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

func (c *compiler) PushScope() *ast.Scope {
	c.scope = ast.NewScope(c.scope)
	return c.scope
}

func (c *compiler) PopScope() *ast.Scope {
	scope := c.scope
	c.scope = c.scope.Outer
	return scope
}

///////////////////////////////////////////////////////////////////////////////

func createPackageMap(pkg *ast.Package) map[*ast.Object]string {
	pkgmap := make(map[*ast.Object]string)
	for _, obj := range pkg.Scope.Objects {
		pkgmap[obj] = pkg.Name
	}
	for _, pkgobj := range pkg.Imports {
		pkgname := pkgobj.Name
		scope := pkgobj.Data.(*ast.Scope)
		for _, obj := range scope.Objects {
			pkgmap[obj] = pkgname
		}
	}
	return pkgmap
}

///////////////////////////////////////////////////////////////////////////////

func NewCompiler() Compiler {
	compiler := new(compiler)
	return compiler
}

func (c *compiler) SetTraceEnabled(enabled bool) {
	if enabled {
		c.logger = log.New(os.Stderr, "", 0)
	} else {
		c.logger = nil
	}
}

func (compiler *compiler) Compile(fset *token.FileSet, pkg *ast.Package) (m *Module, err error) {
	// FIXME create a compilation state, rather than storing in 'compiler'.
	compiler.fileset = fset
	compiler.pkg = pkg
	compiler.initfuncs = make([]Value, 0)

	// Create a Builder, for building LLVM instructions.
	compiler.builder = llvm.GlobalContext().NewBuilder()
	defer compiler.builder.Dispose()

	// Create a Module, which contains the LLVM bitcode. Dispose it on panic,
	// otherwise we'll set a finalizer at the end. The caller may invoke
	// Dispose manually, which will render the finalizer a no-op.
	modulename := pkg.Name
	compiler.module = &Module{llvm.NewModule(modulename), modulename, false}
	compiler.target = llvm.NewTargetData(compiler.module.DataLayout())
	defer func() {
		if e := recover(); e != nil {
			compiler.module.Dispose()
			panic(e)
			//err = e.(error)
		}
	}()
	compiler.types = NewTypeMap(compiler.module.Module, compiler.target)

	// Create a mapping from objects back to packages, so we can create the
	// appropriate symbol names.
	compiler.pkgmap = createPackageMap(pkg)

	// Compile each file in the package.
	for _, file := range pkg.Files {
		file.Scope.Outer = pkg.Scope
		compiler.filescope = file.Scope
		compiler.scope = file.Scope
		compiler.fixConstDecls(file)
		compiler.fixMethodDecls(file)
		for _, decl := range file.Decls {
			compiler.VisitDecl(decl)
		}
	}

	// Define intrinsics for use by the runtime: malloc, free, memcpy, etc.
	compiler.defineRuntimeIntrinsics()

	// Create global constructors.
	//
	// XXX When imports are handled, we'll need to defer creating
	//     llvm.global_ctors until we create an executable. This is
	//     due to (a) imports having to be initialised before the
	//     importer, and (b) LLVM having no specified order of
	//     initialisation for ctors with the same priority.
	if len(compiler.initfuncs) > 0 {
		elttypes := []llvm.Type{
			llvm.Int32Type(),
			llvm.PointerType(
				llvm.FunctionType(llvm.VoidType(), nil, false), 0)}
		ctortype := llvm.StructType(elttypes, false)
		ctors := make([]llvm.Value, len(compiler.initfuncs))
		for i, fn := range compiler.initfuncs {
			struct_values := []llvm.Value{
				llvm.ConstInt(llvm.Int32Type(), 1, false),
				fn.LLVMValue()}
			ctors[i] = llvm.ConstStruct(struct_values, false)
		}

		global_ctors_init := llvm.ConstArray(ctortype, ctors)
		global_ctors_var := llvm.AddGlobal(
			compiler.module.Module, global_ctors_init.Type(),
			"llvm.global_ctors")
		global_ctors_var.SetInitializer(global_ctors_init)
		global_ctors_var.SetLinkage(llvm.AppendingLinkage)
	}

	// Create debug metadata.
	compiler.createMetadata()

	return compiler.module, nil
}

// vim: set ft=go :
