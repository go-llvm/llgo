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
	f := c.NamedFunction("runtime.chansend", "func(c, ptr uintptr)")
	c.builder.CreateCall(f, []llvm.Value{v.LLVMValue(), uintptr_}, "")
}

func (v *LLVMValue) chanRecv() *LLVMValue {
	c := v.compiler
	elttyp := v.typ.Underlying().(*types.Chan).Elem()
	ptr := c.builder.CreateAlloca(c.types.ToLLVM(elttyp), "")
	uintptr_ := c.builder.CreatePtrToInt(ptr, c.target.IntPtrType(), "")
	f := c.NamedFunction("runtime.chanrecv", "func(c, ptr uintptr)")
	c.builder.CreateCall(f, []llvm.Value{v.LLVMValue(), uintptr_}, "")
	value := c.builder.CreateLoad(ptr, "")
	return c.NewValue(value, elttyp)
}
