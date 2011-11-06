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
    "llvm"
    "reflect"
)

const (
    StackVar ast.ObjKind = ast.Lbl + iota
)

type Visitor struct {
    builder llvm.Builder
    modulename string
    module llvm.Module
    functions []llvm.Value
    fileset *token.FileSet
    filescope *ast.Scope
    scope *ast.Scope
}

// TODO take a param to say whether the value is assigned to a constant?
func (self *Visitor) Insert(name string, value llvm.Value, stack bool) {
    t := value.Type()
    var obj *ast.Object
    switch t.TypeKind() {
    case llvm.FunctionTypeKind: {
        obj = ast.NewObj(ast.Fun, name)
        obj.Data = value
    }
    default: {
        // TODO differentiate between constant and variable?
        if stack {
            obj = ast.NewObj(StackVar, name)
            ptr := self.builder.CreateAlloca(value.Type(), name)
            self.builder.CreateStore(value, ptr)
            obj.Data = ptr
        } else {
            obj = ast.NewObj(ast.Var, name)
            obj.Data = value
        }
    }
    }
    existing := self.scope.Insert(obj)
    if existing != nil {existing.Data = obj.Data}
}

func (self *Visitor) Lookup(name string) (llvm.Value, *ast.Object) {
    // TODO check for qualified identifiers (x.y), and short-circuit the
    // lookup.
    for scope := self.scope; scope != nil; scope = scope.Outer {
        obj := scope.Lookup(name)
        if obj != nil {
            if obj.Data != nil {
                value, ok := (obj.Data).(llvm.Value)
                if ok {return value, obj}
                panic(fmt.Sprint("Expected llvm.Value, found ", obj.Data))
            } else {
                panic(fmt.Sprint("Object.Data==nil, Object=", obj))
            }
        }
    }
    return llvm.Value{nil}, nil
}

// Store a value in alloca (stack-allocated) memory.
func (self *Visitor) Store(name string, value llvm.Value) bool {
    for scope := self.scope; scope != nil; scope = scope.Outer {
        obj := scope.Lookup(name)
        if obj != nil && obj.Data != nil && obj.Kind == StackVar {
            ptr, ok := (obj.Data).(llvm.Value)
            if ok {
                self.builder.CreateStore(value, ptr)
                return true
            }
        }
        panic(fmt.Sprint("Failed to find stack allocated variable"))
    }
    return false
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
    defer visitor.builder.Dispose()
    visitor.modulename = file.Name.String()
    visitor.module = llvm.NewModule(visitor.modulename)
    defer visitor.module.Dispose()

    // Process imports first.
    for _, importspec := range file.Imports {
        // 
        fmt.Println("Import: ", importspec)
    }

    // Visit each of the top-level declarations.
    for _, decl := range file.Decls {visitor.VisitDecl(decl);}

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

    // Create 
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
        for _, file := range pkg.Files {
            file.Scope.Outer = pkg.Scope
            VisitFile(fset, file)
        }
    }
}

// vim: set ft=go :

