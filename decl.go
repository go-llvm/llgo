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
    "go/ast"
    "go/token"
    "reflect"
    "github.com/axw/gollvm/llvm"
)

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
    defer self.Insert(f.Name.String(), fn, false)

    entry := llvm.AddBasicBlock(fn, "entry")
    self.builder.SetInsertPointAtEnd(entry)

    self.functions = append(self.functions, fn)
    if f.Body != nil {self.VisitBlockStmt(f.Body)}
    self.functions = self.functions[0:len(self.functions)-1]
    fn_type := fn.Type().ReturnType() // fn.Type() is a pointer-to-function

    if fn_type.ReturnType().TypeKind() == llvm.VoidTypeKind {
        last_block := fn.LastBasicBlock()
        lasti := last_block.LastInstruction()
        if lasti.IsNil() || lasti.Opcode() != llvm.Ret {
            // Assume nil return type, AST should be checked first.
            self.builder.CreateRetVoid()
        }
    }
    return fn
}

func (self *Visitor) VisitGenDecl(decl *ast.GenDecl) {
    switch decl.Tok {
    case token.IMPORT: // No-op (handled in VisitFile
    case token.TYPE: {
        panic("Unhandled type declaration");
    }
    case token.CONST: {
        // Create a new scope to put 'iota' in.
        self.PushScope()
        defer self.PopScope()

        var expr_valspec *ast.ValueSpec
        for iota_, spec := range decl.Specs {
            valspec, ok := spec.(*ast.ValueSpec)
            if !ok {panic("Expected *ValueSpec")}

            // Set iota.
            iota_value := llvm.ConstInt(llvm.Int32Type(), uint64(iota_), false)
            self.Insert("iota", iota_value, false)

            // If no expression is supplied, then use the last expression.
            if valspec.Values != nil && len(valspec.Values) > 0 {
                expr_valspec = valspec
            }

            // TODO value type

            for i, name := range valspec.Names {
                value := self.VisitExpr(expr_valspec.Values[i])
                name_ := name.String()
                if name_ != "_" {
                    obj := self.LookupObj(name_)
                    if obj == nil {panic("Constant lookup failed: " + name_)}
                    obj.Data = value
                }
            }
        }
    }
    case token.VAR: {
        //for _, spec := range decl.Specs {
        //    valuespec, ok := spec.(*ast.ValueSpec)
        //    if !ok {panic("Expected *ValueSpec")}
        //    self.VisitValueSpec(valuespec, isconst)
        //}
    }
    }
}

func (self *Visitor) VisitDecl(decl ast.Decl) llvm.Value {
    switch x := decl.(type) {
    case *ast.FuncDecl: return self.VisitFuncDecl(x)
    case *ast.GenDecl: {
        self.VisitGenDecl(x)
        return llvm.Value{nil}
    }
    }
    panic(fmt.Sprintf("Unhandled decl (%s) at %s\n",
                      reflect.TypeOf(decl),
                      self.fileset.Position(decl.Pos())))
}

// vim: set ft=go :

