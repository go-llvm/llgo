// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"fmt"
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

func (c *compiler) VisitSelectStmt(stmt *ast.SelectStmt) {
	// TODO optimisations:
	//     1. No clauses: runtime.block.
	//     2. Single recv, and default clause: runtime.selectnbrecv
	//     2. Single send, and default clause: runtime.selectnbsend

	startBlock := c.builder.GetInsertBlock()
	function := startBlock.Parent()
	endBlock := llvm.AddBasicBlock(function, "end")
	endBlock.MoveAfter(startBlock)
	defer c.builder.SetInsertPointAtEnd(endBlock)

	f := c.NamedFunction("runtime.selectsize", "func(size int32) uintptr")
	size := llvm.ConstInt(llvm.Int32Type(), uint64(len(stmt.Body.List)), false)
	selectsize := c.builder.CreateCall(f, []llvm.Value{size}, "")
	selectp := c.builder.CreateArrayAlloca(llvm.Int8Type(), selectsize, "selectp")
	c.memsetZero(selectp, selectsize)
	selectp = c.builder.CreatePtrToInt(selectp, c.target.IntPtrType(), "")

	// Cache runtime functions
	var selectrecv, selectsend llvm.Value
	getselectsend := func() llvm.Value {
		if selectsend.IsNil() {
			selectsend = c.NamedFunction("runtime.selectsend", "func(selectp, blockaddr, ch, elem unsafe.Pointer)")
		}
		return selectsend
	}
	getselectrecv := func() llvm.Value {
		if selectrecv.IsNil() {
			selectrecv = c.NamedFunction("runtime.selectrecv", "func(selectp, blockaddr, ch, elem unsafe.Pointer, received *bool)")
		}
		return selectrecv
	}

	for _, stmt := range stmt.Body.List {
		clause := stmt.(*ast.CommClause)
		block := llvm.InsertBasicBlock(endBlock, "")
		c.builder.SetInsertPointAtEnd(block)
		// TODO set Value for case's assigned variables, if any.
		for _, stmt := range clause.Body {
			c.VisitStmt(stmt)
		}
		c.maybeImplicitBranch(endBlock)
		blockaddr := llvm.BlockAddress(function, block)
		blockaddr = c.builder.CreatePtrToInt(blockaddr, c.target.IntPtrType(), "")

		c.builder.SetInsertPointAtEnd(startBlock)
		switch comm := clause.Comm.(type) {
		case nil:
			// default clause
			f := c.NamedFunction("runtime.selectdefault", "func(selectp, blockaddr unsafe.Pointer)")
			c.builder.CreateCall(f, []llvm.Value{selectp, blockaddr}, "")

		case *ast.SendStmt:
			// c<- val
			ch := c.VisitExpr(comm.Chan).LLVMValue()
			elem := c.VisitExpr(comm.Value).LLVMValue()
			f := getselectsend()
			c.builder.CreateCall(f, []llvm.Value{selectp, blockaddr, ch, elem}, "")

		case *ast.ExprStmt:
			// <-c
			ch := c.VisitExpr(comm.X.(*ast.UnaryExpr).X).LLVMValue()
			f := getselectrecv()
			paramtypes := f.Type().ElementType().ParamTypes()
			elem := llvm.ConstNull(paramtypes[2])
			received := llvm.ConstNull(paramtypes[3])
			c.builder.CreateCall(f, []llvm.Value{selectp, blockaddr, ch, elem, received}, "")

		case *ast.AssignStmt:
			// val := <-c
			// val, ok = <-c
			// FIXME handle "_"
			elem := c.VisitExpr(comm.Lhs[0]).(*LLVMValue).pointer.LLVMValue()
			var received llvm.Value
			if len(comm.Lhs) == 2 {
				received = c.VisitExpr(comm.Lhs[1]).(*LLVMValue).pointer.LLVMValue()
			}
			ch := c.VisitExpr(comm.Rhs[0].(*ast.UnaryExpr).X).LLVMValue()
			f := getselectrecv()
			c.builder.CreateCall(f, []llvm.Value{selectp, blockaddr, ch, elem, received}, "")

		default:
			panic(fmt.Errorf("unhandled: %T", comm))
		}
	}

	f = c.NamedFunction("runtime.selectgo", "func(selectp unsafe.Pointer) unsafe.Pointer")
	blockaddr := c.builder.CreateCall(f, []llvm.Value{selectp}, "")
	blockaddr = c.builder.CreateIntToPtr(blockaddr, llvm.PointerType(llvm.Int8Type(), 0), "")
	c.builder.CreateIndirectBr(blockaddr, len(stmt.Body.List))
}
