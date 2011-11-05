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
    "llvm"
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

func (self *Visitor) VisitGenDecl(decl *ast.GenDecl) llvm.Value {
    switch decl.Tok {
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
    return llvm.Value{nil}
}

func (self *Visitor) VisitDecl(decl ast.Decl) llvm.Value {
    switch x := decl.(type) {
    case *ast.FuncDecl: return self.VisitFuncDecl(x)
    case *ast.GenDecl: return self.VisitGenDecl(x)
    }
    panic(fmt.Sprintf("Unhandled decl (%s) at %s\n",
                      reflect.TypeOf(decl),
                      self.fileset.Position(decl.Pos())))
}

// vim: set ft=go :

