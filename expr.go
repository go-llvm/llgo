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
    //"go/token"
    "reflect"
    "github.com/axw/gollvm/llvm"
)

func isglobal(value Value) bool {
    //return !value.IsAGlobalVariable().IsNil()
    return false
}

func isindirect(value Value) bool {
    //return !value.Metadata(llvm.MDKindID("indirect")).IsNil()
    return false
}

func setindirect(value Value) {
    //value.SetMetadata(llvm.MDKindID("indirect"),
    //                  llvm.ConstAllOnes(llvm.Int1Type()))
}

func (self *Visitor) VisitBinaryExpr(expr *ast.BinaryExpr) Value {
    panic("unimplemented")
}

func (self *Visitor) VisitUnaryExpr(expr *ast.UnaryExpr) Value {
    panic("unimplemented")
}

/*
func (self *Visitor) VisitBinaryExpr(expr *ast.BinaryExpr) llvm.Value {
    x := self.VisitExpr(expr.X)
    y := self.VisitExpr(expr.Y)

    // If either is a const and the other is not, then cast the constant to the
    // other's type (to support untyped literals/expressions).
    x_const, y_const := x.IsConstant(), y.IsConstant()
    if x_const && !y_const {
        if isglobal(x) {x = x.Initializer()}
        if isindirect(y) {y = self.builder.CreateLoad(y, "")}
        x = self.maybeCast(x, y.Type())
    } else if !x_const && y_const {
        if isglobal(y) {y = y.Initializer()}
        if isindirect(x) {x = self.builder.CreateLoad(x, "")}
        y = self.maybeCast(y, x.Type())
    } else if x_const && y_const {
        // If either constant is a global variable, 'dereference' it by taking
        // its initializer, which will never change.
        if isglobal(x) {x = x.Initializer()}
        if isglobal(y) {y = y.Initializer()}
        // XXX temporary fix; we should be using exp/types/Const.
        y = self.maybeCast(y, x.Type())
    } else {
        if isindirect(x) {x = self.builder.CreateLoad(x, "")}
        if isindirect(y) {y = self.builder.CreateLoad(y, "")}
    }

    // TODO check types/sign, use float operators if appropriate.
    switch expr.Op {
    case token.MUL:
        if x_const && y_const {
            return llvm.ConstMul(x, y)
        } else {
            return self.builder.CreateMul(x, y, "")
        }
    case token.QUO:
        if x_const && y_const {
            return llvm.ConstUDiv(x, y)
        } else {
            return self.builder.CreateUDiv(x, y, "")
        }
    case token.ADD:
        if x_const && y_const {
            return llvm.ConstAdd(x, y)
        } else {
            return self.builder.CreateAdd(x, y, "")
        }
    case token.SUB:
        if x_const && y_const {
            return llvm.ConstSub(x, y)
        } else {
            return self.builder.CreateSub(x, y, "")
        }
    case token.EQL:
        if x_const && y_const {
            return llvm.ConstICmp(llvm.IntEQ, x, y)
        } else {
            return self.builder.CreateICmp(llvm.IntEQ, x, y, "")
        }
    case token.LSS:
        if x_const && y_const {
            return llvm.ConstICmp(llvm.IntULT, x, y)
        } else {
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
            value = self.builder.CreateNeg(value, "")
        }
    }
    case token.ADD: // No-op
    default: panic("Unhandled operator: ")// + expr.Op)
    }
    return value
}
*/

func (self *Visitor) VisitCallExpr(expr *ast.CallExpr) Value {
    var fn Value
    switch x := (expr.Fun).(type) {
    case *ast.Ident:
        switch x.String() {
        case "println": return self.VisitPrintln(expr)
        case "len": return self.VisitLen(expr)
        case "new": return self.VisitNew(expr)
        default:
            // Is it a type? Then this is a conversion (e.g. int(123))
            if expr.Args != nil && len(expr.Args) == 1 {
                typ := self.GetType(x)
                if typ != nil {
                    value := self.VisitExpr(expr.Args[0])
                    return value.Convert(typ)
                }
            }

            fn = self.Resolve(x.Obj)
            if fn == nil {
                panic(fmt.Sprintf(
                    "No function found with name '%s'", x.String()))
            }
        }
    default:
        fn = self.VisitExpr(expr.Fun)
    }

    //if isindirect(fn) {
    //    fn = self.builder.CreateLoad(fn, "")
    //}

    // Is it a method call? We'll extract the receiver from metadata here,
    // and add it in as the first argument later.
    //receiver := fn.Metadata(llvm.MDKindID("receiver")) // TODO
    receiver := llvm.Value{nil}

    // TODO handle varargs
    var args []llvm.Value = nil
    if expr.Args != nil {
        arg_offset := 0
        if !receiver.IsNil() {
            arg_offset++
            args = make([]llvm.Value, len(expr.Args)+1)
            args[0] = receiver
        } else {
            args = make([]llvm.Value, len(expr.Args))
        }

        fn_type := fn.Type().(*Func)
        param_types := fn_type.Params
        for i, expr := range expr.Args {
            value := self.VisitExpr(expr)
            param_type := param_types[arg_offset+i].Type.(Type)
            args[arg_offset+i] = value.Convert(param_type).LLVMValue()
        }
    } else if !receiver.IsNil() {
        args = []llvm.Value{receiver}
    }
    return NewLLVMValue(self.builder,
        self.builder.CreateCall(fn.LLVMValue(), args, ""))
}

func (self *Visitor) VisitIndexExpr(expr *ast.IndexExpr) Value {
    value := self.VisitExpr(expr.X)
    // TODO handle maps, strings, slices.

    index := self.VisitExpr(expr.Index)
    // TODO
    //if isindirect(index) {index = self.builder.CreateLoad(index, "")}
    isint := false
    if basic, isbasic := index.Type().(*Basic); isbasic {
        switch basic.Kind {
        case Uint8: fallthrough
        case Uint16: fallthrough
        case Uint32: fallthrough
        case Uint64: fallthrough
        case Int8: fallthrough
        case Int16: fallthrough
        case Int32: fallthrough
        case Int64: fallthrough
        case UntypedInt: isint = true
        }
    }
    if !isint {panic("Array index expression must evaluate to an integer")}

    // Is it an array? Then let's get the address of the array so we can
    // get an element.
    // TODO
    //if value.Type().TypeKind() == llvm.ArrayTypeKind {
    //    value = value.Metadata(llvm.MDKindID("address"))
    //}

    zero := llvm.ConstInt(llvm.Int32Type(), 0, false)
    element := self.builder.CreateGEP(
        value.LLVMValue(), []llvm.Value{zero, index.LLVMValue()}, "")
    result := self.builder.CreateLoad(element, "")
    return NewLLVMValue(self.builder, result)
}

func (self *Visitor) VisitSelectorExpr(expr *ast.SelectorExpr) Value {
    lhs := self.VisitExpr(expr.X)
    if lhs == nil {
        // The only time we should get a nil result is if the object is a
        // package.
        pkgident := (expr.X).(*ast.Ident)
        pkgscope := (pkgident.Obj.Data).(*ast.Scope)
        obj := pkgscope.Lookup(expr.Sel.String())
        return self.Resolve(obj)
    }

    // TODO handle interfaces.

    // TODO when we support embedded types, we'll need to do a breadth-first
    // search for the name, since the specification says to take the shallowest
    // field with the specified name.

    // Map name to an index.
    zero_value := llvm.ConstInt(llvm.Int32Type(), 0, false)
    indexes := make([]llvm.Value, 0)
    indexes = append(indexes, zero_value)

    // TODO
/*
    element_type := lhs.Type() //lhs.Type().ElementType()
    if element_type.TypeKind() == llvm.PointerTypeKind {
        indexes = append(indexes, zero_value)
        element_type = element_type.ElementType()
    }

    typeinfo := self.typeinfo[element_type.C]
    index, ok := typeinfo.FieldIndex(expr.Sel.String())
    if ok {
        index_value := llvm.ConstInt(llvm.Int32Type(), uint64(index), false)
        indexes = append(indexes, index_value)
        if lhs.Type().TypeKind() == llvm.PointerTypeKind {
            value := self.builder.CreateGEP(lhs, indexes, "")
            setindirect(value)
            return value
        }
        panic("Don't know how to extract from a register-based struct")
    } else {
        method_obj := typeinfo.MethodByName(expr.Sel.String())
        if method_obj != nil {
            method := self.Resolve(method_obj)
            // TODO
            //method.SetMetadata(llvm.MDKindID("receiver"), lhs)
            return method
        } else {
            panic("Failed to locate field or method: " + expr.Sel.String())
        }
    }
    //return llvm.Value{nil}
*/
    return nil
}

func (self *Visitor) VisitStarExpr(expr *ast.StarExpr) Value {
    // Are we dereferencing a pointer that's on the stack? Then load the stack
    // value.
    operand := self.VisitExpr(expr.X)
    // TODO
    //if isindirect(operand) {
    //    operand = self.builder.CreateLoad(operand, "")
    //}

    // We don't want to immediately load the value, as we might be doing an
    // assignment rather than an evaluation. Instead, we return the pointer and
    // tell the caller to load it on demand.
    setindirect(operand)
    return operand
}

func (self *Visitor) VisitExpr(expr ast.Expr) Value {
    switch x:= expr.(type) {
    case *ast.BasicLit: return self.VisitBasicLit(x)
    case *ast.BinaryExpr: return self.VisitBinaryExpr(x)
    case *ast.FuncLit: return self.VisitFuncLit(x)
    case *ast.CompositeLit: return self.VisitCompositeLit(x)
    case *ast.UnaryExpr: return self.VisitUnaryExpr(x)
    case *ast.CallExpr: return self.VisitCallExpr(x)
    case *ast.IndexExpr: return self.VisitIndexExpr(x)
    case *ast.SelectorExpr: return self.VisitSelectorExpr(x)
    case *ast.StarExpr: return self.VisitStarExpr(x)
    case *ast.Ident: {
        if x.Obj == nil {x.Obj = self.LookupObj(x.Name)}
        return self.Resolve(x.Obj)
    }
    }
    panic(fmt.Sprintf("Unhandled Expr node: %s", reflect.TypeOf(expr)))
}

// vim: set ft=go :

