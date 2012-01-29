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

package llgo

import (
    "fmt"
    "strconv"
    "unsafe"
    "go/ast"
    "go/token"
)

func (self *compiler) VisitLen(expr *ast.CallExpr) Value {
    if len(expr.Args) > 1 {panic("Expecting only one argument to len")}

    value := self.VisitExpr(expr.Args[0])
    switch typ := (value.Type()).(type) {
    case *Pointer:
        // XXX Converting to a string to be converted back to an int is silly.
        // The values need an overhaul? Perhaps have types based on fundamental
        // types, with the additional methods to make them llgo.Value's.
        if a, isarray := typ.Base.(*Array); isarray {
            return NewConstValue(token.INT, strconv.Uitoa64(a.Len))
        }
        return NewConstValue(token.INT,
            strconv.Uitoa(uint(unsafe.Sizeof(uintptr(0)))))
    case *Array:
        // FIXME should be int (arch), not int32.
        return NewConstValue(token.INT, strconv.Uitoa64(typ.Len))
    }
    panic(fmt.Sprint("Unhandled value type: ", value.Type()))
}

// vim: set ft=go :

