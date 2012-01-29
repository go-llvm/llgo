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
    "github.com/axw/gollvm/llvm"
)

func (self *Visitor) VisitBasicLit(lit *ast.BasicLit) Value {
    return NewConstValue(lit.Kind, lit.Value)
}

func (self *Visitor) VisitFuncLit(lit *ast.FuncLit) Value {
    fn_type := self.VisitFuncType(lit.Type)
    fn := llvm.AddFunction(self.module, "", fn_type.LLVMType())
    fn.SetFunctionCallConv(llvm.FastCallConv)

    defer self.builder.SetInsertPointAtEnd(self.builder.GetInsertBlock())
    entry := llvm.AddBasicBlock(fn, "entry")
    self.builder.SetInsertPointAtEnd(entry)

    fn_value := NewLLVMValue(self.builder, fn)
    self.functions = append(self.functions, fn_value)
    self.VisitBlockStmt(lit.Body)
    if fn_type.Results == nil {
        lasti := entry.LastInstruction()
        if lasti.IsNil() || lasti.Opcode() != llvm.Ret {
            // Assume nil return type, AST should be checked first.
            self.builder.CreateRetVoid()
        }
    }
    self.functions = self.functions[0:len(self.functions)-1]
    return fn_value
}

// XXX currently only handles composite array literals
func (self *Visitor) VisitCompositeLit(lit *ast.CompositeLit) Value {
    typ := self.GetType(lit.Type)
    var values []Value
    if lit.Elts != nil {
        valuemap := make(map[int]Value)
        maxi := 0
        for i, elt := range lit.Elts {
            var value Value
            if kv, iskv := elt.(*ast.KeyValueExpr); iskv {
                key := self.VisitExpr(kv.Key)
                i = -1
                if const_key, isconst := key.(*ConstValue); isconst {
                    i = int(const_key.Int64())
                }
                value = self.VisitExpr(kv.Value)
            } else {
                value = self.VisitExpr(elt)
            }
            if i >= 0 {
                if i > maxi {maxi = i}
                valuemap[i] = value
            } else {
                panic("array index must be non-negative integer constant")
            }
        }
        values = make([]Value, maxi+1)
        for i, value := range valuemap {
            values[i] = value
        }
    }

    switch typ_ := typ.(type) {
    case *Array: {
        elttype := typ_.Elt
        llvm_values := make([]llvm.Value, len(values))
        for i, value := range values {
            if value == nil {
                llvm_values[i] = llvm.ConstNull(elttype.LLVMType())
            } else {
                llvm_values[i] = value.Convert(elttype).LLVMValue()
            }
        }
        return NewLLVMValue(self.builder,
            llvm.ConstArray(elttype.LLVMType(), llvm_values))
    }
    }
    panic(fmt.Sprint("Unhandled type kind: ", typ))
}

// vim: set ft=go :

