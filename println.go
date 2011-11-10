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

func (self *Visitor) VisitPrintln(expr *ast.CallExpr) llvm.Value {
    var args []llvm.Value = nil
    var format string
    if expr.Args != nil {
        format = ""
        args = make([]llvm.Value, len(expr.Args)+1)
        for i, expr := range expr.Args {
            value := self.VisitExpr(expr)

            // Is it a global variable or non-constant? Then we'll need to load
            // it if it's not a pointer to an array.
            if isindirect(value) {
                value = self.builder.CreateLoad(value, "")
            }

            if i > 0 {format += " "}
            switch kind := value.Type().TypeKind(); kind {
            case llvm.IntegerTypeKind: {
                switch width := value.Type().IntTypeWidth(); width {
                case 16: format += "%hd"
                case 32: format += "%d"
                case 64: format += "%lld" // FIXME windows
                default: panic(fmt.Sprint("Unhandled integer width ", width))
                }
            }
            case llvm.ArrayTypeKind: {
                // If we see a constant array, we either:
                //     Create an internal constant if it's a constant array, or
                //     Create space on the stack and store it there.
                init_ := value
                if value.IsConstant() {
                    value = llvm.AddGlobal(self.module, init_.Type(), "")
                    value.SetInitializer(init_)
                    value.SetGlobalConstant(true)
                    value.SetLinkage(llvm.InternalLinkage)
                } else {
                    value = self.builder.CreateAlloca(init_.Type(), "")
                    self.builder.CreateStore(init_, value)
                }
                fallthrough
            }
            case llvm.PointerTypeKind: {
                // FIXME don't assume string...
                // TODO string should be a struct, with length & ptr. We'll
                // probably encode the type as metadata.
                format += "%s"
            }
            default: {panic(fmt.Sprint("Unhandled type kind: ", kind))}
            }
            args[i+1] = value
        }
        format += "\n"
    } else {
        args = make([]llvm.Value, 1)
        format = "\n"
    }
    args[0] = self.builder.CreateGlobalStringPtr(format, "")

    printf := getprintf(self.module)
    return self.builder.CreateCall(printf, args, "")
}

// vim: set ft=go :

