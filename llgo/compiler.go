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
    "go/ast"
    "go/token"
    "os"
    "runtime"
    "github.com/axw/gollvm/llvm"
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

type compiler struct {
    builder    llvm.Builder
    module     *Module
    functions  []Value //[]llvm.Value
    initfuncs  []Value //[]llvm.Value
    typeinfo   map[interface{}]*TypeInfo
    imports    map[string]*ast.Object
    fileset    *token.FileSet
    filescope  *ast.Scope
    scope      *ast.Scope
}

func (c *compiler) LookupObj(name string) *ast.Object {
    // TODO check for qualified identifiers (x.y), and short-circuit the
    // lookup.
    for scope := c.scope; scope != nil; scope = scope.Outer {
        obj := scope.Lookup(name)
        if obj != nil {return obj}
    }
    return nil
}

func (c *compiler) Resolve(obj *ast.Object) Value {
    if obj.Kind == ast.Pkg {return nil}
    value, isvalue := (obj.Data).(Value)

    switch obj.Kind {
    case ast.Con:
        if !isvalue {
            valspec := obj.Decl.(*ast.ValueSpec)
            c.VisitValueSpec(valspec, true)
            value, isvalue = (obj.Data).(Value)
        }
    case ast.Fun:
        if !isvalue {
            funcdecl := obj.Decl.(*ast.FuncDecl)
            value = c.VisitFuncProtoDecl(funcdecl)
            obj.Data = value
            isvalue = true
        }
    case ast.Var:
        if !isvalue {
            switch x := (obj.Decl).(type) {
            case *ast.ValueSpec:
                c.VisitValueSpec(x, false)
            case *ast.Field:
                // No-op. Fields will be yielded for function arg/recv/ret.
                // We update the .Data field of the object when we enter the
                // function definition.
            }
            value, isvalue = (obj.Data).(Value)
        }
    }

    if !isvalue {
        panic(fmt.Sprint("Expected Value, found ", obj.Data))
    }
    return value
}

func (c *compiler) Lookup(name string) (Value, *ast.Object) {
    obj := c.LookupObj(name)
    if obj != nil {return c.Resolve(obj), obj}
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

func Compile(fset *token.FileSet, file *ast.File) (m *Module, err os.Error) {
    compiler := new(compiler)
    compiler.fileset = fset
    compiler.filescope = file.Scope
    compiler.scope = file.Scope
    compiler.initfuncs = make([]Value, 0)
    compiler.typeinfo = make(map[interface{}]*TypeInfo)
    compiler.imports = make(map[string]*ast.Object)

    // Create a Builder, for building LLVM instructions.
    compiler.builder = llvm.GlobalContext().NewBuilder()
    defer compiler.builder.Dispose()

    // Create a Module, which contains the LLVM bitcode. Dispose it on panic,
    // otherwise we'll set a finalizer at the end. The caller may invoke
    // Dispose manually, which will render the finalizer a no-op.
    modulename := file.Name.String()
    compiler.module = &Module{llvm.NewModule(modulename), modulename, false}
    defer func() {
        if e := recover(); e != nil {
            compiler.module.Dispose()
            panic(e)
        }
    }()

    // Perform fixups.
    compiler.fixConstDecls(file)
    compiler.fixMethodDecls(file)

    // Visit each of the top-level declarations.
    for _, decl := range file.Decls {compiler.VisitDecl(decl);}

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

    // Create debug metadata. TODO
    //compiler.createCompileUnitMetadata()

    runtime.SetFinalizer(compiler.module, func(m *Module){m.Dispose()})
    return compiler.module, nil
}

// vim: set ft=go :

