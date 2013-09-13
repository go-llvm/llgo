// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"fmt"
	"github.com/axw/gollvm/llvm"
	"go/ast"
	"go/token"
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

	// We create a pointer-pointer for each newly defined var on the
	// lhs of receive expressions, which will be assigned to when the
	// expressions are (conditionally) evaluated.
	lhsptrs := make([][]llvm.Value, len(stmt.Body.List))
	for i, stmt := range stmt.Body.List {
		clause := stmt.(*ast.CommClause)
		if stmt, ok := clause.Comm.(*ast.AssignStmt); ok && stmt.Tok == token.DEFINE {
			lhs := make([]llvm.Value, len(stmt.Lhs))
			for i, expr := range stmt.Lhs {
				ident := expr.(*ast.Ident)
				if !isBlank(ident.Name) {
					typ := c.target.IntPtrType()
					lhs[i] = c.builder.CreateAlloca(typ, "")
					c.builder.CreateStore(llvm.ConstNull(typ), lhs[i])
				}
			}
			lhsptrs[i] = lhs
		}
	}

	// Create clause basic blocks.
	blocks := make([]llvm.BasicBlock, len(stmt.Body.List))
	var basesize uint64
	for i, stmt := range stmt.Body.List {
		clause := stmt.(*ast.CommClause)
		if clause.Comm == nil {
			basesize++
		}
		currBlock := c.builder.GetInsertBlock()
		block := llvm.InsertBasicBlock(endBlock, "")
		c.builder.SetInsertPointAtEnd(block)
		blocks[i] = block
		lhs := lhsptrs[i]
		if stmt, ok := clause.Comm.(*ast.AssignStmt); ok {
			for i, expr := range stmt.Lhs {
				ident := expr.(*ast.Ident)
				if !isBlank(ident.Name) {
					ptr := c.builder.CreateLoad(lhs[i], "")
					obj := c.typeinfo.Objects[ident]
					ptrtyp := types.NewPointer(obj.Type())
					ptr = c.builder.CreateIntToPtr(ptr, c.types.ToLLVM(ptrtyp), "")
					value := c.NewValue(ptr, ptrtyp).makePointee()
					c.objectdata[obj].Value = value
				}
			}
		}
		for _, stmt := range clause.Body {
			c.VisitStmt(stmt)
		}
		c.maybeImplicitBranch(endBlock)
		c.builder.SetInsertPointAtEnd(currBlock)
	}

	// We need to make an initial pass through the cases,
	// discarding those where the channel is nil.
	size := llvm.ConstInt(llvm.Int32Type(), basesize, false)
	channels := make([]Value, len(stmt.Body.List))
	rhs := make([]*LLVMValue, len(stmt.Body.List))
	for i, stmt := range stmt.Body.List {
		clause := stmt.(*ast.CommClause)
		switch comm := clause.Comm.(type) {
		case nil:
		case *ast.SendStmt:
			channels[i] = c.VisitExpr(comm.Chan)
			rhs[i] = c.VisitExpr(comm.Value).(*LLVMValue)
		case *ast.ExprStmt:
			channels[i] = c.VisitExpr(comm.X.(*ast.UnaryExpr).X)
		case *ast.AssignStmt:
			channels[i] = c.VisitExpr(comm.Rhs[0].(*ast.UnaryExpr).X)
		default:
			panic(fmt.Errorf("unhandled: %T", comm))
		}
		if channels[i] != nil {
			nonnil := c.builder.CreateIsNotNull(channels[i].LLVMValue(), "")
			zero := llvm.ConstInt(llvm.Int32Type(), 0, false)
			one := llvm.ConstInt(llvm.Int32Type(), 1, false)
			addend := c.builder.CreateSelect(nonnil, one, zero, "")
			size = c.builder.CreateAdd(size, addend, "")
		}
	}

	f := c.NamedFunction("runtime.selectsize", "func(size int32) uintptr")
	selectsize := c.builder.CreateCall(f, []llvm.Value{size}, "")
	selectp := c.builder.CreateArrayAlloca(llvm.Int8Type(), selectsize, "selectp")
	c.memsetZero(selectp, selectsize)
	selectp = c.builder.CreatePtrToInt(selectp, c.target.IntPtrType(), "")
	f = c.NamedFunction("runtime.initselect", "func(size int32, ptr unsafe.Pointer)")
	c.builder.CreateCall(f, []llvm.Value{size, selectp}, "")

	for i, stmt := range stmt.Body.List {
		clause := stmt.(*ast.CommClause)
		blockaddr := llvm.BlockAddress(function, blocks[i])
		blockaddr = c.builder.CreatePtrToInt(blockaddr, c.target.IntPtrType(), "")
		if clause.Comm == nil {
			// default clause
			f := c.NamedFunction("runtime.selectdefault", "func(selectp, blockaddr unsafe.Pointer)")
			c.builder.CreateCall(f, []llvm.Value{selectp, blockaddr}, "")
			continue
		}

		currBlock := c.builder.GetInsertBlock()
		nextBlock := llvm.InsertBasicBlock(currBlock, "")
		nextBlock.MoveAfter(currBlock)
		block := llvm.InsertBasicBlock(endBlock, "")
		chanval := channels[i].LLVMValue()
		nonnilchan := c.builder.CreateIsNotNull(chanval, "")
		c.builder.CreateCondBr(nonnilchan, block, nextBlock)
		c.builder.SetInsertPointAtEnd(block)

		switch comm := clause.Comm.(type) {
		case *ast.SendStmt:
			// c <- val
			elem := rhs[i]
			var elemptr llvm.Value
			if elem.pointer == nil {
				value := elem.LLVMValue()
				elemptr = c.builder.CreateAlloca(value.Type(), "")
				c.builder.CreateStore(value, elemptr)
			} else {
				elemptr = elem.pointer.LLVMValue()
			}
			elemptr = c.builder.CreatePtrToInt(elemptr, c.target.IntPtrType(), "")
			f := getselectsend()
			c.builder.CreateCall(f, []llvm.Value{selectp, blockaddr, chanval, elemptr}, "")

		case *ast.ExprStmt:
			// <-c
			f := getselectrecv()
			paramtypes := f.Type().ElementType().ParamTypes()
			elem := llvm.ConstNull(paramtypes[3])
			received := llvm.ConstNull(paramtypes[4])
			c.builder.CreateCall(f, []llvm.Value{selectp, blockaddr, chanval, elem, received}, "")

		case *ast.AssignStmt:
			// val := <-c
			// val, ok = <-c
			f := getselectrecv()
			lhs := c.assignees(comm)
			paramtypes := f.Type().ElementType().ParamTypes()
			var elem llvm.Value
			if lhs[0] != nil {
				elem = lhs[0].pointer.LLVMValue()
				elem = c.builder.CreatePtrToInt(elem, paramtypes[3], "")
				if !lhsptrs[i][0].IsNil() {
					c.builder.CreateStore(elem, lhsptrs[i][0])
				}
			} else {
				elem = llvm.ConstNull(paramtypes[3])
			}
			var received llvm.Value
			if len(lhs) == 2 && lhs[1] != nil {
				received = lhs[1].pointer.LLVMValue()
				received = c.builder.CreatePtrToInt(received, paramtypes[4], "")
				if !lhsptrs[i][1].IsNil() {
					c.builder.CreateStore(received, lhsptrs[i][1])
				}
			} else {
				received = llvm.ConstNull(paramtypes[4])
			}
			c.builder.CreateCall(f, []llvm.Value{selectp, blockaddr, chanval, elem, received}, "")
		}

		c.builder.CreateBr(nextBlock)
		c.builder.SetInsertPointAtEnd(nextBlock)
	}

	f = c.NamedFunction("runtime.selectgo", "func(selectp unsafe.Pointer) unsafe.Pointer")
	blockaddr := c.builder.CreateCall(f, []llvm.Value{selectp}, "")
	blockaddr = c.builder.CreateIntToPtr(blockaddr, llvm.PointerType(llvm.Int8Type(), 0), "")
	ibr := c.builder.CreateIndirectBr(blockaddr, len(stmt.Body.List))
	for _, block := range blocks {
		ibr.AddDest(block)
	}
}
