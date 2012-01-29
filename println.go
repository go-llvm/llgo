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

func getprintf(module llvm.Module) llvm.Value {
    printf := module.NamedFunction("printf")
    if printf.IsNil() {
        CharPtr := llvm.PointerType(llvm.Int8Type(), 0)
        fn_type := llvm.FunctionType(
            llvm.Int32Type(), []llvm.Type{CharPtr}, true)
        printf = llvm.AddFunction(module, "printf", fn_type)
        printf.SetFunctionCallConv(llvm.CCallConv)
    }
    return printf
}

func (self *Visitor) VisitPrintln(expr *ast.CallExpr) Value {
    var args []llvm.Value = nil
    var format string
    if expr.Args != nil {
        format = ""
        args = make([]llvm.Value, len(expr.Args)+1)
        for i, expr := range expr.Args {
            value := self.VisitExpr(expr)
            llvm_value := value.LLVMValue()

            // Is it a global variable or non-constant? Then we'll need to load
            // it if it's not a pointer to an array.
            if isindirect(value) {
                // TODO
                //value = self.builder.CreateLoad(value, "")
            }

            if i > 0 {format += " "}
            switch typ := (value.Type()).(type) {
            case *Basic:
                switch typ.Kind {
                case Int: format += "%d" // TODO 32/64
                case Int16: format += "%hd"
                case Int32: format += "%d"
                case Int64: format += "%lld" // FIXME windows
                case String:
                    // Hrm. This kinda sucks. What's the appropriate way to
                    // automatically convert constant strings to globals?
                    if !llvm_value.IsAConstant().IsNil() &&
                       llvm_value.IsAGlobalValue().IsNil() {
                        g := llvm.AddGlobal(self.module, llvm_value.Type(), "")
                        g.SetInitializer(llvm_value)
                        g.SetGlobalConstant(true)
                        g.SetLinkage(llvm.InternalLinkage)
                        llvm_value = g
                    }
                    format += "%s"
                default: panic(fmt.Sprint("Unhandled Basic Kind: ", typ.Kind))
                }

            //case *Slice: fallthrough
            case *Slice, *Array:
                // If we see a constant array, we either:
                //     Create an internal constant if it's a constant array, or
                //     Create space on the stack and store it there.
                init_ := value
                init_value := init_.LLVMValue()
                switch init_.(type) {
                case ConstValue:
                    llvm_value = llvm.AddGlobal(
                        self.module, init_value.Type(), "")
                    llvm_value.SetInitializer(init_value)
                    llvm_value.SetGlobalConstant(true)
                    llvm_value.SetLinkage(llvm.InternalLinkage)
                case *LLVMValue:
                    llvm_value = self.builder.CreateAlloca(
                        init_value.Type(), "")
                    self.builder.CreateStore(init_value, llvm_value)
                }
                // FIXME don't assume string...
                format += "%s"

            case *Pointer:
                // FIXME don't assume string...
                // TODO string should be a struct, with length & ptr. We'll
                // probably encode the type as metadata.
                format += "%s"
            default: panic(fmt.Sprint("Unhandled type kind"))
            }

            args[i+1] = llvm_value
        }
        format += "\n"
    } else {
        args = make([]llvm.Value, 1)
        format = "\n"
    }
    args[0] = self.builder.CreateGlobalStringPtr(format, "")

    printf := getprintf(self.module)
    return NewLLVMValue(
        self.builder, self.builder.CreateCall(printf, args, ""))
}

// vim: set ft=go :

