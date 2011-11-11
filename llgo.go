/*
Copyright (c) 2011 Andrew Wilkins <axwalk@gmail.com>

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

package main

import (
    "fmt"
    "flag"
    "go/parser"
    "go/ast"
    "go/token"
    "go/types"
    "os"
    "reflect"
    "github.com/axw/gollvm/llvm"
)

type Visitor struct {
    builder llvm.Builder
    modulename string
    module llvm.Module
    functions []llvm.Value
    initfuncs []llvm.Value
    fileset *token.FileSet
    filescope *ast.Scope
    scope *ast.Scope
}

func (self *Visitor) LookupObj(name string) *ast.Object {
    // TODO check for qualified identifiers (x.y), and short-circuit the
    // lookup.
    for scope := self.scope; scope != nil; scope = scope.Outer {
        obj := scope.Lookup(name)
        if obj != nil {return obj}
    }
    return nil
}

func (self *Visitor) Resolve(obj *ast.Object) llvm.Value {
    value, isvalue := (obj.Data).(llvm.Value)

    switch obj.Kind {
    case ast.Con:
        if !isvalue {
            valspec, _ := (obj.Decl).(*ast.ValueSpec)
            self.VisitValueSpec(valspec, true)
            value, isvalue = (obj.Data).(llvm.Value)
        }
    case ast.Fun:
        if !isvalue {
            funcdecl, _ := (obj.Decl).(*ast.FuncDecl)
            value = self.VisitFuncProtoDecl(funcdecl)
            obj.Data = value
            isvalue = true
        }
    case ast.Var:
        if !isvalue {
            valspec, _ := (obj.Decl).(*ast.ValueSpec)
            self.VisitValueSpec(valspec, false)
            value, isvalue = (obj.Data).(llvm.Value)
        }
    }

    if !isvalue {
        panic(fmt.Sprint("Expected llvm.Value, found ", obj.Data))
    }
    return value
}

func (self *Visitor) Lookup(name string) (llvm.Value, *ast.Object) {
    obj := self.LookupObj(name)
    if obj != nil {return self.Resolve(obj), obj}
    return llvm.Value{nil}, nil
}

func (self *Visitor) PushScope() *ast.Scope {
    self.scope = ast.NewScope(self.scope)
    return self.scope
}

func (self *Visitor) PopScope() *ast.Scope {
    scope := self.scope
    self.scope = self.scope.Outer
    return scope
}

///////////////////////////////////////////////////////////////////////////////

func (self *Visitor) maybeCast(value llvm.Value, totype llvm.Type) llvm.Value {
    value_type := value.Type()
    switch value_type.TypeKind() {
    case llvm.IntegerTypeKind: {
        switch totype.TypeKind() {
        case llvm.IntegerTypeKind: {
            delta := value_type.IntTypeWidth()-totype.IntTypeWidth()
            switch {
            case delta == 0: return value
            // TODO handle signed/unsigned (SExt/ZExt)
            case delta < 0: return self.builder.CreateZExt(value, totype, "")
            case delta > 0: return self.builder.CreateTrunc(value, totype, "")
            }
        }
        }
    }
/*
    case llvm.FloatTypeKind: {
        switch to_
    }
    case llvm.DoubleTypeKind: {
        
    }
*/
    }
    return value
}


///////////////////////////////////////////////////////////////////////////////

func (self *Visitor) IdentGetType(ident *ast.Ident) llvm.Type {
    switch ident.Name {
        case "bool": return llvm.Int1Type()

        // TODO do we use 'metadata' to mark a type as un/signed?
        case "uint": fallthrough
        case "int": return llvm.Int32Type() // TODO 32/64 depending on arch

        case "byte": fallthrough
        case "uint8": fallthrough
        case "int8": return llvm.Int8Type()

        case "uint16": fallthrough
        case "int16": return llvm.Int16Type()

        case "uint32": fallthrough
        case "int32": return llvm.Int32Type()

        case "uint64": fallthrough
        case "int64": return llvm.Int64Type()

        case "float32": return llvm.FloatType()
        case "float64": return llvm.DoubleType()

        //case "complex64": fallthrough
        //case "complex128": fallthrough
    }

    // XXX if ast/types were done, and we had ast.Package
    // with Scope set, we could just get x.Obj here?
    obj := self.filescope.Lookup(ident.String())
    if obj != nil {
        switch x := (obj.Decl).(type) {
        case *ast.TypeSpec: {
            panic("TypeSpec handling incomplete")
        }
        default: {
            panic("Unhandled type")
        }
        }
    }
    return llvm.Type{nil}
}

func (self *Visitor) GetType(expr ast.Expr) llvm.Type {
    switch x := (expr).(type) {
    case *ast.Ident:
        return self.IdentGetType(x)
    case *ast.FuncType:
        type_ := self.VisitFuncType(x)
        type_ = llvm.PointerType(type_, 0)
        return type_
    case *ast.ArrayType:
        var len_ int = -1
        if x.Len == nil {panic("Unhandled slice ArrayType")}
        elttype := self.GetType(x.Elt)
        _, isellipsis := (x.Len).(*ast.Ellipsis)
        if !isellipsis {
            lenvalue := self.VisitExpr(x.Len)
            if lenvalue.IsAConstantInt().IsNil() {
                panic("Array length must be a constant integer expression")
            }
            len_ = int(lenvalue.ZExtValue())
        }
        return llvm.ArrayType(elttype, len_)
    default:
        panic(fmt.Sprint("Unhandled Expr: ", reflect.TypeOf(x)))
    }
    return llvm.Type{nil}
}

func (self *Visitor) VisitFuncType(f *ast.FuncType) llvm.Type {
    var fn_args []llvm.Type = nil
    var fn_rettype llvm.Type

    // TODO process args

    if f.Results == nil || f.Results.List == nil {
        fn_rettype = llvm.VoidType()
    } else {
        for i := 0; i < len(f.Results.List); i++ {
            fn_rettype = self.GetType(f.Results.List[i].Type)
        }
    }
    return llvm.FunctionType(fn_rettype, fn_args, false)
}

///////////////////////////////////////////////////////////////////////////////

var dump *bool = flag.Bool(
                    "dump", false,
                    "Dump the AST to stderr instead of generating bitcode")

func VisitFile(fset *token.FileSet, file *ast.File) {
    visitor := new(Visitor)
    visitor.fileset = fset
    visitor.filescope = file.Scope
    visitor.scope = file.Scope
    visitor.builder = llvm.GlobalContext().NewBuilder()
    visitor.initfuncs = make([]llvm.Value, 0)
    defer visitor.builder.Dispose()
    visitor.modulename = file.Name.String()
    visitor.module = llvm.NewModule(visitor.modulename)
    defer visitor.module.Dispose()

    // Process imports first.
    for _, importspec := range file.Imports {
        // 
        fmt.Println("Import: ", importspec)
    }

    // Perform fixups.
    fixConstDecls(file)

    // Visit each of the top-level declarations.
    for _, decl := range file.Decls {visitor.VisitDecl(decl);}

    // Create global constructors.
    //
    // XXX When imports are handled, we'll need to defer creating
    //     llvm.global_ctors until we create an executable. This is
    //     due to (a) imports having to be initialised before the
    //     importer, and (b) LLVM having no specified order of
    //     initialisation for ctors with the same priority.
    if len(visitor.initfuncs) > 0 {
        elttypes := []llvm.Type{
            llvm.Int32Type(),
            llvm.FunctionType(llvm.VoidType(), nil, false)}
        ctortype := llvm.StructType(elttypes, false)
        ctors := make([]llvm.Value, len(visitor.initfuncs))
        for i, fn := range visitor.initfuncs {
            struct_values := []llvm.Value{
                llvm.ConstInt(llvm.Int32Type(), 1, false), fn}
            ctors[i] = llvm.ConstStruct(struct_values, false)
        }

        global_ctors_init := llvm.ConstArray(ctortype, ctors)
        global_ctors_var := llvm.AddGlobal(
            visitor.module, global_ctors_init.Type(), "llvm.global_ctors")
        global_ctors_var.SetInitializer(global_ctors_init)
        global_ctors_var.SetLinkage(llvm.AppendingLinkage)
    }

    if *dump {
        visitor.module.Dump()
    } else {
        err := llvm.WriteBitcodeToFile(visitor.module, os.Stdout)
        if err != nil {fmt.Println(err)}
    }
}

func main() {
    flag.Parse()
    fset := token.NewFileSet()

    filenames := flag.Args()
    packages, err := parser.ParseFiles(fset, filenames, 0)
    if err != nil {
        fmt.Printf("ParseFiles failed: %s\n", err.String())
        os.Exit(1)
    }

    // Resolve imports.
/*
    imports := make(map[string]*ast.Object)
    for _, pkg := range packages {
        types.GcImporter(imports, path)
    }
*/

    // Create a new scope for each package.
    for _, pkg := range packages {
        pkg.Scope = ast.NewScope(types.Universe)
        obj := ast.NewObj(ast.Pkg, pkg.Name)
        obj.Data = pkg.Scope
    }

    // Type check and fill in the AST.
    for _, pkg := range packages {
        // TODO Imports? Or will 'Check' fill it in?
        types.Check(fset, pkg)
        //fmt.Println(pkg.Imports)
    }

    // Build an LLVM module.
    for _, pkg := range packages {
        file := ast.MergePackageFiles(pkg, 0)
        file.Scope = ast.NewScope(pkg.Scope)
        VisitFile(fset, file)
    }
}

// vim: set ft=go :

