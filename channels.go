// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
	"go/ast"
)

func (c *compiler) VisitSendStmt(stmt *ast.SendStmt) {
	channel := c.VisitExpr(stmt.Chan).(*LLVMValue)
	value := c.VisitExpr(stmt.Value)
	channel.chanSend(value)
}

func (c *compiler) visitClose(expr *ast.CallExpr) {
	channel := c.VisitExpr(expr.Args[0]).(*LLVMValue)
	channel.chanClose()
}

func (v *LLVMValue) chanClose() {
	f := v.compiler.NamedFunction("runtime.chanclose", "func(c uintptr)")
	v.compiler.builder.CreateCall(f, []llvm.Value{v.LLVMValue()}, "")
}

func (v *LLVMValue) chanSend(value Value) {
	var ptr llvm.Value
	if value, ok := value.(*LLVMValue); ok && value.pointer != nil {
		ptr = value.pointer.LLVMValue()
	}
	elttyp := v.typ.Underlying().(*types.Chan).Elem()
	c := v.compiler
	if ptr.IsNil() {
		ptr = c.builder.CreateAlloca(c.types.ToLLVM(elttyp), "")
		value := value.Convert(elttyp).LLVMValue()
		c.builder.CreateStore(value, ptr)
	}
	uintptr_ := c.builder.CreatePtrToInt(ptr, c.target.IntPtrType(), "")
	nb := boolLLVMValue(false)
	f := c.NamedFunction("runtime.chansend", "func(t *chanType, c, ptr uintptr, nb bool) bool")
	chantyp := c.types.ToRuntime(v.typ.Underlying())
	chantyp = c.builder.CreateBitCast(chantyp, f.Type().ElementType().ParamTypes()[0], "")
	c.builder.CreateCall(f, []llvm.Value{chantyp, v.LLVMValue(), uintptr_, nb}, "")
	// Ignore result; only used in runtime.
}

func (v *LLVMValue) chanRecv(commaok bool) (value, received *LLVMValue) {
	c := v.compiler
	elttyp := v.typ.Underlying().(*types.Chan).Elem()
	ptr := c.builder.CreateAlloca(c.types.ToLLVM(elttyp), "")
	uintptr_ := c.builder.CreatePtrToInt(ptr, c.target.IntPtrType(), "")
	nb := boolLLVMValue(false)
	f := c.NamedFunction("runtime.chanrecv", "func(t *chanType, c, ptr uintptr, nb bool) (received bool)")
	chantyp := c.types.ToRuntime(v.typ.Underlying())
	chantyp = c.builder.CreateBitCast(chantyp, f.Type().ElementType().ParamTypes()[0], "")
	callresult := c.builder.CreateCall(f, []llvm.Value{chantyp, v.LLVMValue(), uintptr_, nb}, "")
	llvmvalue := c.builder.CreateLoad(ptr, "")
	if commaok {
		received = c.NewValue(callresult, types.Typ[types.Bool])
	}
	return c.NewValue(llvmvalue, elttyp), received
}
