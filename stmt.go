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

package main

import (
    "fmt"
    "go/ast"
    "go/token"
    "reflect"
    "github.com/axw/gollvm/llvm"
)

func (self *Visitor) VisitIncDecStmt(stmt *ast.IncDecStmt) {
    ptr := self.VisitExpr(stmt.X)
    value := self.builder.CreateLoad(ptr.LLVMValue(), "")
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
    self.builder.CreateStore(value, ptr.LLVMValue())
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
            //cur_fn := self.functions[len(self.functions)-1]
            //fn_type := cur_fn.Type()

            // TODO Convert value to the function's return type.
            result := value

            self.builder.CreateRet(result.LLVMValue())
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
    values := make([]Value, len(stmt.Rhs))
    for i, expr := range stmt.Rhs {values[i] = self.VisitExpr(expr)}
    for i, expr := range stmt.Lhs {
        value := values[i]
        switch x := expr.(type) {
        case *ast.Ident: {
            if x.Name != "_" {
                obj := x.Obj
                if stmt.Tok == token.DEFINE {
                    ptr := self.builder.CreateAlloca(
                        value.Type().LLVMType(), x.Name)
                    //setindirect(ptr) TODO
                    self.builder.CreateStore(value.LLVMValue(), ptr)
                    obj.Data = ptr
                } else {
                    ptr, _ := (obj.Data).(Value)
                    value = value.Convert(Deref(ptr.Type()))
                    self.builder.CreateStore(
                        value.LLVMValue(), ptr.LLVMValue())
                }
            }
        }
        default:
            ptr := self.VisitExpr(expr)
            value = value.Convert(Deref(ptr.Type()))
            self.builder.CreateStore(value.LLVMValue(), ptr.LLVMValue())
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
    self.builder.CreateCondBr(
        cond_val.LLVMValue(), if_block, else_block)

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
        self.builder.CreateCondBr(
            cond_val.LLVMValue(), loop_block, done_block)
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

func (self *Visitor) VisitGoStmt(stmt *ast.GoStmt) {
    //stmt.Call *ast.CallExpr
    // TODO 
    var fn Value
    switch x := (stmt.Call.Fun).(type) {
    case *ast.Ident:
        fn = self.Resolve(x.Obj)
        if fn == nil {
            panic(fmt.Sprintf(
                "No function found with name '%s'", x.String()))
        }
    default:
        fn = self.VisitExpr(stmt.Call.Fun)
    }

    // Evaluate arguments, store in a structure on the stack.
    var args_struct_type llvm.Type
    var args_mem llvm.Value
    var args_size llvm.Value
    if stmt.Call.Args != nil {
        param_types := make([]llvm.Type, 0)
        fn_type, _ := (fn.Type()).(*Func)
        for _, param := range fn_type.Params {
            typ := param.Type.(Type)
            param_types = append(param_types, typ.LLVMType())
        }
        args_struct_type = llvm.StructType(param_types, false)
        args_mem = self.builder.CreateAlloca(args_struct_type, "")
        for i, expr := range stmt.Call.Args {
            value_i := self.VisitExpr(expr)
            value_i = value_i.Convert(fn_type.Params[i].Type.(Type))
            arg_i := self.builder.CreateGEP(args_mem, []llvm.Value{
                    llvm.ConstInt(llvm.Int32Type(), 0, false),
                    llvm.ConstInt(llvm.Int32Type(), uint64(i), false)}, "")
            // TODO
            //if isindirect(value_i) {
            //    value_i = self.builder.CreateLoad(value_i, "")
            //}
            self.builder.CreateStore(value_i.LLVMValue(), arg_i)
        }
        args_size = llvm.SizeOf(args_struct_type)
    } else {
        args_struct_type = llvm.VoidType()
        args_mem = llvm.ConstNull(llvm.PointerType(args_struct_type, 0))
        args_size = llvm.ConstInt(llvm.Int32Type(), 0, false)
    }

    // When done, return to where we were.
    defer self.builder.SetInsertPointAtEnd(self.builder.GetInsertBlock())

    // Create a function that will take a pointer to a structure of the type
    // defined above, or no parameters if there are none to pass.
    indirect_fn_type := llvm.FunctionType(
        llvm.VoidType(),
        []llvm.Type{llvm.PointerType(args_struct_type, 0)}, false)
    indirect_fn := llvm.AddFunction(self.module, "", indirect_fn_type)
    indirect_fn.SetFunctionCallConv(llvm.CCallConv)

    // Call "newgoroutine" with the indirect function and stored args.
    newgoroutine := getnewgoroutine(self.module)
    ngr_param_types := newgoroutine.Type().ElementType().ParamTypes()
    fn_arg := self.builder.CreateBitCast(indirect_fn, ngr_param_types[0], "")
    args_arg := self.builder.CreateBitCast(args_mem,
        llvm.PointerType(llvm.Int8Type(), 0), "")
    self.builder.CreateCall(newgoroutine,
        []llvm.Value{fn_arg, args_arg, args_size}, "")

    entry := llvm.AddBasicBlock(indirect_fn, "entry")
    self.builder.SetInsertPointAtEnd(entry)
    var args []llvm.Value
    if stmt.Call.Args != nil {
        args_mem = indirect_fn.Param(0)
        args = make([]llvm.Value, len(stmt.Call.Args))
        for i := range stmt.Call.Args {
            arg_i := self.builder.CreateGEP(args_mem, []llvm.Value{
                       llvm.ConstInt(llvm.Int32Type(), 0, false),
                       llvm.ConstInt(llvm.Int32Type(), uint64(i), false)}, "")
            args[i] = self.builder.CreateLoad(arg_i, "")
        }
    }
    self.builder.CreateCall(fn.LLVMValue(), args, "")
    self.builder.CreateRetVoid()
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
    case *ast.DeclStmt: self.VisitDecl(x.Decl)
    case *ast.GoStmt: self.VisitGoStmt(x)
    default: panic(fmt.Sprintf("Unhandled Stmt node: %s", reflect.TypeOf(stmt)))
    }
}

// vim: set ft=go :

