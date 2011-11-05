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

func (self *Visitor) VisitIncDecStmt(stmt *ast.IncDecStmt) {
    value := self.VisitExpr(stmt.X)
    one := llvm.ConstInt(value.Type(), 1, false)

    switch stmt.Tok {
    case token.INC: {value = self.builder.CreateAdd(value, one, "")}
    case token.DEC: {value = self.builder.CreateSub(value, one, "")}
    }

    // TODO make sure we cover all possibilities (maybe just delegate this to
    // an assignment statement handler, and do it all in one place).
    //
    // In the case of a simple variable, we simply calculate the new value and
    // update the value in the scope.
    switch x := stmt.X.(type) {
    case *ast.Ident: {
        if !self.Store(x.Name, value) {
            panic(fmt.Sprintf("No object found with name '%s'", x.Name))
        }
        return
    }
    }
    panic(fmt.Sprint("Unhandled Expr node: ", reflect.TypeOf(stmt.X)))
}

func (self *Visitor) VisitBlockStmt(stmt *ast.BlockStmt) {
    self.PushScope()
    defer self.PopScope()
    for _, stmt := range stmt.List {
        self.VisitStmt(stmt)
    }
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
                if stmt.Tok == token.DEFINE {
                    self.Insert(x.Name, value, true)
                } else {
                    self.Store(x.Name, value)
                }
            }
        }
        default: panic("Unhandled assignment")
        }
    }
}

func (self *Visitor) VisitIfStmt(stmt *ast.IfStmt) {
    curr_block := self.builder.GetInsertBlock()
    if_block := llvm.AddBasicBlock(curr_block.Parent(), "if")
    else_block := llvm.AddBasicBlock(curr_block.Parent(), "else")
    resume_block := llvm.AddBasicBlock(curr_block.Parent(), "endif")

    if stmt.Init != nil {
        self.PushScope()
        self.VisitStmt(stmt.Init)
        defer self.PopScope()
    }

    cond_val := self.VisitExpr(stmt.Cond)
    self.builder.CreateCondBr(cond_val, if_block, else_block)

    self.builder.SetInsertPointAtEnd(if_block)
    self.VisitBlockStmt(stmt.Body)
    self.builder.CreateBr(resume_block)

    self.builder.SetInsertPointAtEnd(else_block)
    if stmt.Else != nil {self.VisitStmt(stmt.Else)}
    self.builder.CreateBr(resume_block)
}

func (self *Visitor) VisitForStmt(stmt *ast.ForStmt) {
    curr_block := self.builder.GetInsertBlock()
    var cond_block, loop_block, done_block llvm.BasicBlock
    if stmt.Cond != nil {
        cond_block = llvm.AddBasicBlock(curr_block.Parent(), "cond")
    }
    loop_block = llvm.AddBasicBlock(curr_block.Parent(), "loop")
    done_block = llvm.AddBasicBlock(curr_block.Parent(), "done")

    // Is there an initializer? Create a new scope and visit the statement.
    if stmt.Init != nil {
        self.PushScope()
        self.VisitStmt(stmt.Init)
        defer self.PopScope()
    }

    // Start the loop.
    if stmt.Cond != nil {
        self.builder.CreateBr(cond_block)
        self.builder.SetInsertPointAtEnd(cond_block)
        cond_val := self.VisitExpr(stmt.Cond)
        self.builder.CreateCondBr(cond_val, loop_block, done_block)
    } else {
        self.builder.CreateBr(loop_block)
    }

    // Loop body.
    self.builder.SetInsertPointAtEnd(loop_block)
    self.VisitBlockStmt(stmt.Body)
    if stmt.Post != nil {self.VisitStmt(stmt.Post)}
    if stmt.Cond != nil {
        self.builder.CreateBr(cond_block)
    } else {
        self.builder.CreateBr(loop_block)
    }
    self.builder.SetInsertPointAtEnd(done_block)
}

func (self *Visitor) VisitStmt(stmt ast.Stmt) {
    switch x := stmt.(type) {
    case *ast.ReturnStmt: self.VisitReturnStmt(x)
    case *ast.AssignStmt: self.VisitAssignStmt(x)
    case *ast.IncDecStmt: self.VisitIncDecStmt(x)
    case *ast.IfStmt: self.VisitIfStmt(x)
    case *ast.ForStmt: self.VisitForStmt(x)
    case *ast.ExprStmt: self.VisitExpr(x.X)
    case *ast.BlockStmt: self.VisitBlockStmt(x)
    default: panic(fmt.Sprintf("Unhandled Stmt node: %s", reflect.TypeOf(stmt)))
    }
}

// vim: set ft=go :

