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
    "strconv"
    "github.com/axw/gollvm/llvm"
)

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
    case token.STRING: {
        s, err := strconv.Unquote(lit.Value)
        if err != nil {panic(err)}
        return self.builder.CreateGlobalStringPtr(s, "")
    }
    }
    panic("Unhandled BasicLit node")
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

func (self *Visitor) VisitCompositeLit(lit *ast.CompositeLit) llvm.Value {
    type_ := self.GetType(lit.Type)
    var values []llvm.Value
    if lit.Elts != nil {
        values = make([]llvm.Value, len(lit.Elts))
        for i, elt := range lit.Elts {values[i] = self.VisitExpr(elt)}
    }

    switch type_.TypeKind() {
    case llvm.ArrayTypeKind: {
        elttype := type_.ElementType()
        for i, value := range values {
            values[i] = self.maybeCast(value, elttype)
        }
        return llvm.ConstArray(elttype, values)
    }
    }
    panic(fmt.Sprint("Unhandled type kind: ", type_.TypeKind()))
}

// vim: set ft=go :

