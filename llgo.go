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
    "os"
    "llvm"
    "reflect"
)

type Visitor struct {
    builder llvm.Builder
    module llvm.Module
    fileset *token.FileSet
    filescope *ast.Scope
    locals map[string] llvm.Value
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

func (self *Visitor) VisitUnaryExpr(expr *ast.UnaryExpr) llvm.Value {
    value := self.VisitExpr(expr.X)
    if !value.IsNil() {
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
    }
    return value
}

func (self *Visitor) VisitExpr(expr ast.Expr) llvm.Value {
    switch x:= expr.(type) {
    case *ast.BasicLit: return self.VisitBasicLit(x)
    case *ast.UnaryExpr: return self.VisitUnaryExpr(x)
    case *ast.Ident: {
        // TODO use Scope?
        return self.locals[x.Name]
    }
    }
    panic(fmt.Sprintf("Unhandled Expr node: %s", reflect.TypeOf(expr)))
}

///////////////////////////////////////////////////////////////////////////////

func (self *Visitor) VisitBlockStmt(stmt *ast.BlockStmt) {
    for _, stmt := range stmt.List {
        self.VisitStmt(stmt)
    }
}

func (self *Visitor) VisitReturnStmt(stmt *ast.ReturnStmt) {
    if stmt.Results == nil {
        self.builder.CreateRetVoid()
    } else {
        if len(stmt.Results) == 1 {
            self.builder.CreateRet(self.VisitExpr(stmt.Results[0]))
        } else {
            for _, expr := range stmt.Results {
                self.VisitExpr(expr)
            }
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
                self.locals[x.Name] = value
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
    default: panic(fmt.Sprintf("Unhandled Stmt node: %s", reflect.TypeOf(stmt)))
    }
}

///////////////////////////////////////////////////////////////////////////////

func (self *Visitor) GetType(ident *ast.Ident) llvm.Type {
    switch ident.Name {
        case "int": return llvm.Int32Type()
    }

    // XXX if ast/types were done, and we had ast.Package
    // with Scope set, we could just get x.Obj here?
    obj := self.filescope.Lookup(ident.String())
    if obj == nil {
        panic(fmt.Sprintf("Failed lookup on identifier '%s'", ident))
    }

    switch x := (obj.Decl).(type) {
        case *ast.TypeSpec: {
        }
        default: {
            panic("Unhandled type")
        }
    }
    return llvm.Type{nil}
}

func (self *Visitor) VisitFuncDecl(f *ast.FuncDecl) *llvm.Value {
    var fn_args []llvm.Type = nil
    var fn_rettype llvm.Type
    //fn_args = []llvm.Type{llvm.Int32Type()}

    if f.Type.Results == nil || f.Type.Results.List == nil {
        fn_rettype = llvm.VoidType()
    } else {
        for i := 0; i < len(f.Type.Results.List); i++ {
            //ident := ast.Ident(f.Type.Results.List[i].Type)
            //obj := self.filescope.Lookup(ident.String())
            switch x := (f.Type.Results.List[i].Type).(type) {
            case *ast.Ident:
                fn_rettype = self.GetType(x)
            default:
                fmt.Println("??????")
            }
        }
    }

    // Create a new map for named values in this function.
    // FIXME make this stack based, for lexical scoping. Can we
    // reuse ast.Scope?
    self.locals = make(map[string] llvm.Value)

    // Create the function.
    fn_type := llvm.FunctionType(fn_rettype, fn_args, false)
    fn := llvm.AddFunction(self.module, f.Name.String(), fn_type)
    fn.SetFunctionCallConv(llvm.FastCallConv) // XXX
    //n := fac.Param(0)

    entry := llvm.AddBasicBlock(fn, "entry")
    self.builder.SetInsertPointAtEnd(entry)
    if f.Body != nil {
        self.VisitBlockStmt(f.Body)
    }
    return nil
}

func (self *Visitor) VisitDecl(decl ast.Decl) *llvm.Value {
    switch x := decl.(type) {
        case *ast.FuncDecl: {
            return self.VisitFuncDecl(x)
        }
        default: {
            fmt.Printf("Unhandled decl at %s\n",
                self.fileset.Position(decl.Pos()));
        }
    }
    return nil
}

///////////////////////////////////////////////////////////////////////////////

func VisitFile(fset *token.FileSet, file *ast.File) {
    visitor := new(Visitor)
    visitor.fileset = fset
    visitor.filescope = file.Scope
    visitor.builder = llvm.GlobalContext().NewBuilder()
    defer visitor.builder.Dispose()
    visitor.module = llvm.NewModule(file.Name.String())
    defer visitor.module.Dispose()

    for i := 0; i < len(file.Decls); i++ {
        visitor.VisitDecl(file.Decls[i])
    }

    visitor.module.Dump()
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

    // Build an LLVM module.
    VisitFile(fset, ast_)
}

