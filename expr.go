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
    "llvm"
    "reflect"
)

func (self *Visitor) VisitBinaryExpr(expr *ast.BinaryExpr) llvm.Value {
    x := self.VisitExpr(expr.X)
    y := self.VisitExpr(expr.Y)

    // If either is a const and the other is not, then cast the other is not,
    // cast the constant to the other's type.
    x_const, y_const := x.IsConstant(), y.IsConstant()
    if x_const && !y_const {
        x = self.maybeCast(x, y.Type())
    } else if !x_const && y_const {
        y = self.maybeCast(y, x.Type())
    }

    // TODO check types/sign, use float operators if appropriate.
    switch expr.Op {
    case token.MUL: {return self.builder.CreateMul(x, y, "")}
    case token.QUO: {return self.builder.CreateUDiv(x, y, "")}
    case token.ADD: {return self.builder.CreateAdd(x, y, "")}
    case token.SUB: {return self.builder.CreateSub(x, y, "")}
    case token.EQL: {return self.builder.CreateICmp(llvm.IntEQ, x, y, "")}
    case token.LSS: {
        return self.builder.CreateICmp(llvm.IntULT, x, y, "")
    }
    }
    panic(fmt.Sprint("Unhandled operator: ", expr.Op))
}

func (self *Visitor) VisitUnaryExpr(expr *ast.UnaryExpr) llvm.Value {
    value := self.VisitExpr(expr.X)
    switch expr.Op {
    case token.SUB: {
        if !value.IsAConstant().IsNil() {
            value = llvm.ConstNeg(value)
        } else {
            value = self.builder.CreateNeg(value, "negate") // XXX name?
        }
    }
    case token.ADD: {/*No-op*/}
    default: panic("Unhandled operator: ")// + expr.Op)
    }
    return value
}

func (self *Visitor) VisitCallExpr(expr *ast.CallExpr) llvm.Value {
    switch x := (expr.Fun).(type) {
    case *ast.Ident: {
        switch x.String() {
        case "println": {return self.VisitPrintln(expr)}
        case "len": {return self.VisitLen(expr)}
        default: {
            // Is it a type? Then this is a conversion (e.g. int(123))
            if expr.Args != nil && len(expr.Args) == 1 {
                typ := self.GetType(x)
                if !typ.IsNil() {
                    value := self.VisitExpr(expr.Args[0])
                    return self.maybeCast(value, typ)
                }
            }

            fn, obj := self.Lookup(x.String())
            if fn.IsNil() {
                panic(fmt.Sprintf(
                    "No function found with name '%s'", x.String()))
            } else if obj.Kind == StackVar {
                fn = self.builder.CreateLoad(fn, "")
            }

            // TODO handle varargs
            var args []llvm.Value = nil
            if expr.Args != nil {
                args = make([]llvm.Value, len(expr.Args))
                for i, expr := range expr.Args {args[i] = self.VisitExpr(expr)}
            }
            return self.builder.CreateCall(fn, args, "")
        }
        }
    }
    }
    panic("Unhandled CallExpr")
}

func (self *Visitor) VisitIndexExpr(expr *ast.IndexExpr) llvm.Value {
    value := self.VisitExpr(expr.X)
    // TODO handle maps, strings, slices.

    index := self.VisitExpr(expr.Index)
    if index.Type().TypeKind() != llvm.IntegerTypeKind {
        panic("Array index expression must evaluate to an integer")
    }

    // Is it an array? Then let's get the address of the array so we can
    // get an element.
    if value.Type().TypeKind() == llvm.ArrayTypeKind {
        value = value.Metadata(llvm.MDKindID("address"))
    }

    zero := llvm.ConstInt(llvm.Int32Type(), 0, false)
    element := self.builder.CreateGEP(value, []llvm.Value{zero, index}, "")
    return self.builder.CreateLoad(element, "")
}

func (self *Visitor) VisitExpr(expr ast.Expr) llvm.Value {
    switch x:= expr.(type) {
    case *ast.BasicLit: return self.VisitBasicLit(x)
    case *ast.BinaryExpr: return self.VisitBinaryExpr(x)
    case *ast.FuncLit: return self.VisitFuncLit(x)
    case *ast.CompositeLit: return self.VisitCompositeLit(x)
    case *ast.UnaryExpr: return self.VisitUnaryExpr(x)
    case *ast.CallExpr: return self.VisitCallExpr(x)
    case *ast.IndexExpr: return self.VisitIndexExpr(x)
    case *ast.Ident: {
        value, obj := self.Lookup(x.Name)
        if value.IsNil() {
            panic(fmt.Sprintf("No object found with name '%s'", x.Name))
        } else {
            // XXX how do we reverse this if we want to do "&ident"?
            if obj.Kind == StackVar {
                ptrvalue := value
                value = self.builder.CreateLoad(value, "")
                value.SetMetadata(llvm.MDKindID("address"), ptrvalue)
            }
        }
        return value
    }
    }
    panic(fmt.Sprintf("Unhandled Expr node: %s", reflect.TypeOf(expr)))
}

// vim: set ft=go :

