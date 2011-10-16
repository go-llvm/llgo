// vim: set ft=go :
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
func (self *Visitor) Insert(name string, value llvm.Value) {
    t := value.Type()
    var obj *ast.Object
    switch t.TypeKind() {
    case llvm.FunctionTypeKind: {
        obj = ast.NewObj(ast.Fun, name)
        obj.Data = value
    }
    default: {
        // TODO differentiate between constant and variable?
        obj = ast.NewObj(ast.Var, name)
        obj.Data = value
    }
    }
    existing := self.scope.Insert(obj)
    if existing != nil {existing.Data = obj.Data}
}

func (self *Visitor) Lookup(name string) llvm.Value {
    // TODO check for qualified identifiers (x.y), and short-circuit the
    // lookup.
    for scope := self.scope; scope != nil; scope = scope.Outer {
        obj := scope.Lookup(name)
        if obj != nil {
            if obj.Data != nil {
                value, ok := (obj.Data).(llvm.Value)
                if (ok) {return value}
                panic(fmt.Sprint("Expected llvm.Value, found ", obj.Data))
            } else {
                panic(fmt.Sprint("Object.Data==nil, Object=", obj))
            }
        }
    }
    return llvm.Value{nil}
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

func (self *Visitor) VisitBasicLit(lit *ast.BasicLit) llvm.Value {
    switch lit.Kind {
    case token.INT: {
        // XXX how do we determine type properly (size, signedness)? It must
        // be based on the expression/declaration in which it is used? Do
        // we just take the best-fit, and cast as necessary?
        var value uint64
        n, err := fmt.Sscan(lit.Value, &value)
        if err != nil {
            panic(err.String())
        } else if n != 1 {
            panic("Failed to extract integer value")
        }
        return llvm.ConstInt(llvm.Int64Type(), value, false)
    }
    //case token.FLOAT:
    //case token.IMAG:
    //case token.CHAR:
    //case token.STRING:
    }
    panic("Unhandled BasicLit node")
}

func (self *Visitor) VisitBinaryExpr(expr *ast.BinaryExpr) llvm.Value {
    x := self.VisitExpr(expr.X)
    y := self.VisitExpr(expr.Y)

    // If either is a const and the other is not, then cast the other is not,
    // cast the constant to the other's type.
    x_const, y_const := x.IsConstant(), y.IsConstant()
    if x_const && !y_const {
        x = self.maybeCast(x, y.Type())
    } else if !x_const && y_const {
        y = self.maybeCast(y, x.Type())
    }

    // TODO check types, use float operators if appropriate.
    switch expr.Op {
    case token.MUL: {return self.builder.CreateMul(x, y, "")}
    case token.ADD: {return self.builder.CreateMul(x, y, "")}
    }
    panic("Unhandled operator: ")
}

func (self *Visitor) VisitUnaryExpr(expr *ast.UnaryExpr) llvm.Value {
    value := self.VisitExpr(expr.X)
    switch expr.Op {
    case token.SUB: {
        if !value.IsAConstant().IsNil() {
            value = llvm.ConstNeg(value)
        } else {
            value = self.builder.CreateNeg(value, "negate") // XXX name?
        }
    }
    case token.ADD: {/*No-op*/}
    default: panic("Unhandled operator: ")// + expr.Op)
    }
    return value
}

func (self *Visitor) VisitCallExpr(expr *ast.CallExpr) llvm.Value {
    switch x := (expr.Fun).(type) {
    case *ast.Ident: {
        // Is it a type? Then this is a conversion (e.g. int(123))
        if expr.Args != nil && len(expr.Args) == 1 {
            typ := self.GetType(x)
            if !typ.IsNil() {
                value := self.VisitExpr(expr.Args[0])
                return self.maybeCast(value, typ)
            }
        }

        fn := self.Lookup(x.String())
        if fn.IsNil() {
            panic(fmt.Sprintf("No function found with name '%s'", x.String()))
        }

        // TODO handle varargs
        var args []llvm.Value = nil
        if expr.Args != nil {
            args = make([]llvm.Value, len(expr.Args))
            for i, expr := range expr.Args {args[i] = self.VisitExpr(expr)}
        }
        return self.builder.CreateCall(fn, args, "")
    }
    }
    fmt.Println(reflect.TypeOf(expr.Fun), expr)
    panic("Unhandled CallExpr")
}

func (self *Visitor) VisitFuncLit(lit *ast.FuncLit) llvm.Value {
    fn_type := self.VisitFuncType(lit.Type)
    fn := llvm.AddFunction(self.module, "", fn_type)
    fn.SetFunctionCallConv(llvm.FastCallConv)

    defer self.builder.SetInsertPointAtEnd(self.builder.GetInsertBlock())
    entry := llvm.AddBasicBlock(fn, "entry")
    self.builder.SetInsertPointAtEnd(entry)

    self.functions = append(self.functions, fn)
    self.VisitBlockStmt(lit.Body)
    if fn_type.ReturnType().TypeKind() == llvm.VoidTypeKind {
        lasti := entry.LastInstruction()
        if lasti.IsNil() || lasti.Opcode() != llvm.Ret {
            // Assume nil return type, AST should be checked first.
            self.builder.CreateRetVoid()
        }
    }
    self.functions = self.functions[0:len(self.functions)-1]
    return fn
}

func (self *Visitor) VisitExpr(expr ast.Expr) llvm.Value {
    switch x:= expr.(type) {
    case *ast.BasicLit: return self.VisitBasicLit(x)
    case *ast.BinaryExpr: return self.VisitBinaryExpr(x)
    case *ast.FuncLit: return self.VisitFuncLit(x)
    case *ast.UnaryExpr: return self.VisitUnaryExpr(x)
    case *ast.CallExpr: return self.VisitCallExpr(x)
    case *ast.Ident: {
        value := self.Lookup(x.Name)
        if value.IsNil() {
            panic(fmt.Sprintf("No object found with name '%s'", x.Name))
        }
        return value
    }
    }
    panic(fmt.Sprintf("Unhandled Expr node: %s", reflect.TypeOf(expr)))
}

///////////////////////////////////////////////////////////////////////////////

func (self *Visitor) VisitBlockStmt(stmt *ast.BlockStmt) {
    self.PushScope()
    defer self.PopScope()
    for _, stmt := range stmt.List {
        self.VisitStmt(stmt)
    }
}

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

func (self *Visitor) VisitReturnStmt(stmt *ast.ReturnStmt) {
    if stmt.Results == nil {
        self.builder.CreateRetVoid()
    } else {
        if len(stmt.Results) == 1 {
            value := self.VisitExpr(stmt.Results[0])
            cur_fn := self.functions[len(self.functions)-1]
            fn_type := cur_fn.Type().ReturnType()
            return_type := fn_type.ReturnType()
            self.builder.CreateRet(self.maybeCast(value, return_type))
        } else {
            // TODO
            for _, expr := range stmt.Results {
                self.VisitExpr(expr)
            }
            panic("Handling multiple results not yet implemented")
        }
    }
}

func (self *Visitor) VisitAssignStmt(stmt *ast.AssignStmt) {
    values := make([]llvm.Value, len(stmt.Rhs))
    for i, expr := range stmt.Rhs {values[i] = self.VisitExpr(expr)}
    for i, expr := range stmt.Lhs {
        switch x := expr.(type) {
        case *ast.Ident: {
            if x.Name != "_" {
                value := values[i]
                self.Insert(x.Name, value)
            }
        }
        default: panic("Unhandled assignment")
        }
    }
}

func (self *Visitor) VisitStmt(stmt ast.Stmt) {
    switch x := stmt.(type) {
    case *ast.ReturnStmt: self.VisitReturnStmt(x)
    case *ast.AssignStmt: self.VisitAssignStmt(x)
    case *ast.ExprStmt: self.VisitExpr(x.X)
    default: panic(fmt.Sprintf("Unhandled Stmt node: %s", reflect.TypeOf(stmt)))
    }
}

///////////////////////////////////////////////////////////////////////////////

func (self *Visitor) GetType(ident *ast.Ident) llvm.Type {
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
    if obj == nil {
        panic(fmt.Sprintf("Failed lookup on identifier '%s'", ident))
    }

    switch x := (obj.Decl).(type) {
        case *ast.TypeSpec: {
            panic("TypeSpec handling incomplete")
        }
        default: {
            panic("Unhandled type")
        }
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
            switch x := (f.Results.List[i].Type).(type) {
            case *ast.Ident:
                fn_rettype = self.GetType(x)
            case *ast.FuncType:
                fn_rettype = self.VisitFuncType(x)
                fn_rettype = llvm.PointerType(fn_rettype, 0)
            default:
                fmt.Printf("?????? %s\n", reflect.TypeOf(x))
            }
        }
    }
    return llvm.FunctionType(fn_rettype, fn_args, false)
}

func (self *Visitor) VisitFuncProtoDecl(f *ast.FuncDecl) llvm.Value {
    fn_type := self.VisitFuncType(f.Type)
    fn_name := f.Name.String()
    var fn llvm.Value
    if self.modulename == "main" && fn_name == "main" {
        fn = llvm.AddFunction(self.module, "main", fn_type)
        fn.SetLinkage(llvm.ExternalLinkage)
        // TODO store main fn
    } else {
        fn = llvm.AddFunction(self.module, fn_name, fn_type)
        fn.SetFunctionCallConv(llvm.FastCallConv) // XXX
    }
    return fn
}

func (self *Visitor) VisitFuncDecl(f *ast.FuncDecl) llvm.Value {
    fn := self.VisitFuncProtoDecl(f)
    defer self.Insert(f.Name.String(), fn)

    entry := llvm.AddBasicBlock(fn, "entry")
    self.builder.SetInsertPointAtEnd(entry)

    self.functions = append(self.functions, fn)
    if f.Body != nil {self.VisitBlockStmt(f.Body)}
    self.functions = self.functions[0:len(self.functions)-1]
    fn_type := fn.Type().ReturnType() // fn.Type() is a pointer-to-function
    if fn_type.ReturnType().TypeKind() == llvm.VoidTypeKind {
        lasti := entry.LastInstruction()
        if lasti.IsNil() || lasti.Opcode() != llvm.Ret {
            // Assume nil return type, AST should be checked first.
            self.builder.CreateRetVoid()
        }
    }
    return fn
}

func (self *Visitor) VisitDecl(decl ast.Decl) llvm.Value {
    switch x := decl.(type) {
    case *ast.FuncDecl: return self.VisitFuncDecl(x)
    case *ast.GenDecl: {
        switch x.Tok {
        case token.IMPORT: // No-op (handled in VisitFile
        case token.CONST: {
            panic("Unhandled const declaration");
        }
        case token.TYPE: {
            panic("Unhandled type declaration");
        }
        case token.VAR: {
            panic("Unhandled var declaration");
        }
        }
    }
    }
    panic(fmt.Sprintf("Unhandled decl (%s) at %s\n",
                      reflect.TypeOf(decl),
                      self.fileset.Position(decl.Pos())))
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
        fmt.Println(importspec)
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

    filename := flag.Arg(0)
    ast_, err := parser.ParseFile(fset, filename, nil, 0)
    if err != nil {
        fmt.Printf("ParseFile failed: %s\n", err.String())
        os.Exit(1)
    }

    // TODO we should be using the go/types package for type checking, but it's
    // not ready yet.

    // Build an LLVM module.
    VisitFile(fset, ast_)
}

