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
    "llvm"
)

func (self *Visitor) VisitLen(expr *ast.CallExpr) llvm.Value {
    if len(expr.Args) > 1 {panic("Expecting only one argument to len")}

    value := self.VisitExpr(expr.Args[0])
    typ := value.Type()
    switch typ.TypeKind() {
    case llvm.PointerTypeKind: {
        if typ.ElementType().TypeKind() != llvm.ArrayTypeKind {break}
        typ = typ.ElementType()
        fallthrough
    }
    case llvm.ArrayTypeKind: {
        // FIXME should be int (arch), not int32.
        return llvm.ConstInt(
            llvm.Int32Type(), uint64(typ.ArrayLength()), false)
    }
    }
    panic(fmt.Sprint("Unhandled value type: ", value.Type()))
}

// vim: set ft=go :

