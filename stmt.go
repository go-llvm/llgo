// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/exact"
	"code.google.com/p/go.tools/go/types"
	"fmt"
	"github.com/axw/gollvm/llvm"
	"go/ast"
	"go/token"
	"reflect"
)

// maybeImplicitBranch creates a branch from the current position to the
// specified basic block, if and only if the current basic block's last
// instruction is not a terminator.
//
// If dest is nil, then branch to the next basic block, if any.
func (c *compiler) maybeImplicitBranch(dest llvm.BasicBlock) {
	currBlock := c.builder.GetInsertBlock()
	if in := currBlock.LastInstruction(); in.IsNil() || in.IsATerminatorInst().IsNil() {
		if dest.IsNil() {
			dest = llvm.NextBasicBlock(currBlock)
			if !dest.IsNil() {
				c.builder.CreateBr(dest)
			}
		} else {
			c.builder.CreateBr(dest)
		}
	}
}

func (c *compiler) VisitIncDecStmt(stmt *ast.IncDecStmt) {
	lhs := c.VisitExpr(stmt.X).(*LLVMValue)
	rhs := c.NewConstValue(exact.MakeUint64(1), lhs.Type())
	op := token.ADD
	if stmt.Tok == token.DEC {
		op = token.SUB
	}
	result := lhs.BinaryOp(op, rhs)
	c.builder.CreateStore(result.LLVMValue(), lhs.pointer.LLVMValue())

	// TODO make sure we cover all possibilities (maybe just delegate this to
	// an assignment statement handler, and do it all in one place).
	//
	// In the case of a simple variable, we simply calculate the new value and
	// update the value in the scope.
}

func (c *compiler) VisitBlockStmt(stmt *ast.BlockStmt, createNewBlock bool) {
	// This is a little awkward, but it makes dealing with branching easier.
	// A free-standing block statement (i.e. one not attached to a control
	// statement) will splice in a new block.
	var doneBlock llvm.BasicBlock
	if createNewBlock {
		currBlock := c.builder.GetInsertBlock()
		doneBlock = llvm.InsertBasicBlock(currBlock, "")
		doneBlock.MoveAfter(currBlock)
		newBlock := llvm.InsertBasicBlock(doneBlock, "")
		c.builder.CreateBr(newBlock)
		c.builder.SetInsertPointAtEnd(newBlock)
	}

	// Visit each statement in the block. When we have a terminator,
	// ignore everything until we get to a labeled statement.
	for _, stmt := range stmt.List {
		currBlock := c.builder.GetInsertBlock()
		in := currBlock.LastInstruction()
		if in.IsNil() || in.IsATerminatorInst().IsNil() {
			c.VisitStmt(stmt)
		} else if _, ok := stmt.(*ast.LabeledStmt); ok {
			// FIXME we might end up with a labeled statement
			// with no predecessors, due to dead code elimination.
			c.VisitStmt(stmt)
		}
	}

	if createNewBlock {
		c.maybeImplicitBranch(doneBlock)
		c.builder.SetInsertPointAtEnd(doneBlock)
	}
}

func (c *compiler) VisitReturnStmt(stmt *ast.ReturnStmt) {
	f := c.functions.top()
	ftyp := f.Type().(*types.Signature)
	if ftyp.Results().Len() == 0 {
		if !f.deferblock.IsNil() {
			c.builder.CreateBr(f.deferblock)
		} else {
			c.builder.CreateRetVoid()
		}
		return
	}

	// Convert untyped values.
	for i, expr := range stmt.Results {
		if typ := c.typeinfo.Types[expr]; isUntyped(typ) {
			c.typeinfo.Types[expr] = ftyp.Results().At(i).Type()
		}
	}

	values := make([]llvm.Value, int(ftyp.Results().Len()))
	if stmt.Results == nil {
		// Bare return. No need to update named results, so just
		// prepare return values.
		for i := 0; i < int(f.results.Len()); i++ {
			resultvar := f.results.At(i)
			if !isBlank(resultvar.Name()) {
				values[i] = c.objectdata[resultvar].Value.LLVMValue()
			} else {
				typ := c.types.ToLLVM(ftyp.Results().At(i).Type())
				values[i] = llvm.ConstNull(typ)
			}
		}
	} else {
		results := make([]Value, int(ftyp.Results().Len()))
		if len(stmt.Results) == 1 && len(results) > 1 {
			aggresult := c.VisitExpr(stmt.Results[0])
			aggtyp := aggresult.Type().(*types.Tuple)
			aggval := aggresult.LLVMValue()
			for i := 0; i < len(results); i++ {
				elemtyp := aggtyp.At(i).Type()
				elemval := c.builder.CreateExtractValue(aggval, i, "")
				result := c.NewValue(elemval, elemtyp)
				results[i] = result.Convert(ftyp.Results().At(i).Type())
			}
		} else {
			for i, expr := range stmt.Results {
				result := c.VisitExpr(expr)
				results[i] = result.Convert(ftyp.Results().At(i).Type())
			}
		}

		// Convert results to LLVM values.
		for i, result := range results {
			values[i] = result.LLVMValue()
		}

		// Store values in named results.
		if f.results != nil {
			for i := 0; i < int(f.results.Len()); i++ {
				resultvar := f.results.At(i)
				if !isBlank(resultvar.Name()) {
					resultptr := c.objectdata[resultvar].Value.pointer
					c.builder.CreateStore(values[i], resultptr.LLVMValue())
				}
			}
		}
	}

	if !f.deferblock.IsNil() {
		c.builder.CreateBr(f.deferblock)
	} else {
		if len(values) == 1 {
			c.builder.CreateRet(values[0])
		} else {
			c.builder.CreateAggregateRet(values)
		}
	}
}

// nonAssignmentToken returns the non-assignment token
func nonAssignmentToken(t token.Token) token.Token {
	switch t {
	case token.ADD_ASSIGN,
		token.SUB_ASSIGN,
		token.MUL_ASSIGN,
		token.QUO_ASSIGN,
		token.REM_ASSIGN:
		return token.ADD + (t - token.ADD_ASSIGN)
	case token.AND_ASSIGN,
		token.OR_ASSIGN,
		token.XOR_ASSIGN,
		token.SHL_ASSIGN,
		token.SHR_ASSIGN,
		token.AND_NOT_ASSIGN:
		return token.AND + (t - token.AND_ASSIGN)
	}
	return token.ILLEGAL
}

// destructureExpr evaluates the right-hand side of a
// multiple assignment where the right-hand side is a single expression.
func (c *compiler) destructureExpr(x ast.Expr) []Value {
	var values []Value
	switch x := x.(type) {
	case *ast.IndexExpr:
		// value, ok := m[k]
		m := c.VisitExpr(x.X).(*LLVMValue)
		index := c.VisitExpr(x.Index)
		value, notnull := c.mapLookup(m, index, false)
		values = []Value{value, notnull}
	case *ast.CallExpr:
		value := c.VisitExpr(x)
		aggregate := value.LLVMValue()
		struct_type := value.Type().(*types.Tuple)
		values = make([]Value, int(struct_type.Len()))
		for i := range values {
			f := struct_type.At(i)
			t := f.Type()
			value_ := c.builder.CreateExtractValue(aggregate, i, "")
			values[i] = c.NewValue(value_, t)
		}
	case *ast.TypeAssertExpr:
		lhs := c.VisitExpr(x.X).(*LLVMValue)
		typ := c.typeinfo.Types[x.Type]
		switch typ := typ.Underlying().(type) {
		case *types.Interface:
			value, ok := lhs.convertI2I(typ)
			values = []Value{value, ok}
		default:
			value, ok := lhs.convertI2V(typ)
			values = []Value{value, ok}
		}
	}
	return values
}

func (c *compiler) VisitAssignStmt(stmt *ast.AssignStmt) {
	// x (add_op|mul_op)= y
	if stmt.Tok != token.DEFINE && stmt.Tok != token.ASSIGN {
		// TODO handle assignment to map element.
		c.convertUntyped(stmt.Rhs[0], stmt.Lhs[0])
		op := nonAssignmentToken(stmt.Tok)
		lhs := c.VisitExpr(stmt.Lhs[0])
		rhsValue := c.VisitExpr(stmt.Rhs[0])
		rhsValue = rhsValue.Convert(lhs.Type())
		newValue := lhs.BinaryOp(op, rhsValue).(*LLVMValue).LLVMValue()
		c.builder.CreateStore(newValue, lhs.(*LLVMValue).pointer.LLVMValue())
		return
	}

	// a, b, ... [:]= x, y, ...
	var values []Value
	if len(stmt.Rhs) == 1 && len(stmt.Lhs) > 1 {
		values = c.destructureExpr(stmt.Rhs[0])
	} else {
		values = make([]Value, len(stmt.Lhs))
		for i, expr := range stmt.Rhs {
			c.convertUntyped(expr, stmt.Lhs[i])
			values[i] = c.VisitExpr(expr)
		}
	}

	// FIXME must evaluate lhs before evaluating rhs.
	lhsptrs := make([]llvm.Value, len(stmt.Lhs))
	for i, expr := range stmt.Lhs {
		switch x := expr.(type) {
		case *ast.Ident:
			if isBlank(x.Name) {
				continue
			}
			obj := c.typeinfo.Objects[x]
			if stmt.Tok == token.DEFINE {
				typ := obj.Type()
				llvmtyp := c.types.ToLLVM(typ)
				ptr := c.builder.CreateAlloca(llvmtyp, x.Name)
				ptrtyp := types.NewPointer(typ)
				stackvar := c.NewValue(ptr, ptrtyp).makePointee()
				stackvar.stack = c.functions.top().LLVMValue
				c.objectdata[obj].Value = stackvar
				lhsptrs[i] = ptr
				continue
			}
			if c.objectdata[obj].Value == nil {
				// FIXME this is crap, going to need to revisit
				// how decl's are visited (should be in data
				// dependent order.)
				functions := c.functions
				c.functions = nil
				c.VisitValueSpec(x.Obj.Decl.(*ast.ValueSpec))
				c.functions = functions
			}
		case *ast.IndexExpr:
			t := c.typeinfo.Types[x.X]
			if _, ok := t.Underlying().(*types.Map); ok {
				m := c.VisitExpr(x.X).(*LLVMValue)
				index := c.VisitExpr(x.Index)
				elem, _ := c.mapLookup(m, index, true)
				lhsptrs[i] = elem.pointer.LLVMValue()
				values[i] = values[i].Convert(elem.Type())
				continue
			}
		}

		// default (since we can't fallthrough in non-map index exprs)
		lhs := c.VisitExpr(expr).(*LLVMValue)
		lhsptrs[i] = lhs.pointer.LLVMValue()
		values[i] = values[i].Convert(lhs.Type())
	}

	// Must evaluate all of rhs values before assigning.
	llvmvalues := make([]llvm.Value, len(values))
	for i, v := range values {
		if !lhsptrs[i].IsNil() {
			llvmvalues[i] = v.LLVMValue()
		}
	}
	for i, v := range llvmvalues {
		ptr := lhsptrs[i]
		if !ptr.IsNil() {
			c.builder.CreateStore(v, ptr)
		}
	}
}

func (c *compiler) VisitIfStmt(stmt *ast.IfStmt) {
	currBlock := c.builder.GetInsertBlock()
	resumeBlock := llvm.AddBasicBlock(currBlock.Parent(), "endif")
	resumeBlock.MoveAfter(currBlock)
	defer c.builder.SetInsertPointAtEnd(resumeBlock)

	var ifBlock, elseBlock llvm.BasicBlock
	if stmt.Else != nil {
		elseBlock = llvm.InsertBasicBlock(resumeBlock, "else")
		ifBlock = llvm.InsertBasicBlock(elseBlock, "if")
	} else {
		ifBlock = llvm.InsertBasicBlock(resumeBlock, "if")
	}
	if stmt.Else == nil {
		elseBlock = resumeBlock
	}

	if stmt.Init != nil {
		c.VisitStmt(stmt.Init)
	}

	cond_val := c.VisitExpr(stmt.Cond)
	c.builder.CreateCondBr(cond_val.LLVMValue(), ifBlock, elseBlock)
	c.builder.SetInsertPointAtEnd(ifBlock)
	c.VisitBlockStmt(stmt.Body, false)
	c.maybeImplicitBranch(resumeBlock)

	if stmt.Else != nil {
		c.builder.SetInsertPointAtEnd(elseBlock)
		c.VisitStmt(stmt.Else)
		c.maybeImplicitBranch(resumeBlock)
	}
}

func (c *compiler) VisitForStmt(stmt *ast.ForStmt) {
	currBlock := c.builder.GetInsertBlock()
	doneBlock := llvm.AddBasicBlock(currBlock.Parent(), "done")
	doneBlock.MoveAfter(currBlock)
	loopBlock := llvm.InsertBasicBlock(doneBlock, "loop")
	defer c.builder.SetInsertPointAtEnd(doneBlock)

	condBlock := loopBlock
	if stmt.Cond != nil {
		condBlock = llvm.InsertBasicBlock(loopBlock, "cond")
	}

	postBlock := condBlock
	if stmt.Post != nil {
		postBlock = llvm.InsertBasicBlock(doneBlock, "post")
	}

	if c.lastlabel != nil {
		labelData := c.labelData(c.lastlabel)
		labelData.Break = doneBlock
		labelData.Continue = postBlock
		c.lastlabel = nil
	}

	c.breakblocks = append(c.breakblocks, doneBlock)
	c.continueblocks = append(c.continueblocks, postBlock)
	defer func() {
		c.breakblocks = c.breakblocks[:len(c.breakblocks)-1]
		c.continueblocks = c.continueblocks[:len(c.continueblocks)-1]
	}()

	if stmt.Init != nil {
		c.VisitStmt(stmt.Init)
	}

	// Start the loop.
	if stmt.Cond != nil {
		c.builder.CreateBr(condBlock)
		c.builder.SetInsertPointAtEnd(condBlock)
		condVal := c.VisitExpr(stmt.Cond)
		c.builder.CreateCondBr(condVal.LLVMValue(), loopBlock, doneBlock)
	} else {
		c.builder.CreateBr(loopBlock)
	}

	// Post.
	if stmt.Post != nil {
		c.builder.SetInsertPointAtEnd(postBlock)
		c.VisitStmt(stmt.Post)
		c.builder.CreateBr(condBlock)
	}

	// Loop body.
	c.builder.SetInsertPointAtEnd(loopBlock)
	c.VisitBlockStmt(stmt.Body, false)
	c.maybeImplicitBranch(postBlock)
}

func (c *compiler) VisitGoStmt(stmt *ast.GoStmt) {
	fn := c.VisitExpr(stmt.Call.Fun).(*LLVMValue)
	fntype := fn.Type().Underlying().(*types.Signature)
	dotdotdot := stmt.Call.Ellipsis.IsValid()
	args := c.evalCallArgs(fntype, stmt.Call.Args, dotdotdot)
	go_ := c.NamedFunction("runtime.go", "func(f_ func())")
	funcval := c.indirectFunction(fn, args, dotdotdot)
	c.builder.CreateCall(go_, []llvm.Value{funcval.LLVMValue()}, "")
}

func (c *compiler) VisitSwitchStmt(stmt *ast.SwitchStmt) {
	if stmt.Init != nil {
		c.VisitStmt(stmt.Init)
	}

	var tag Value
	if stmt.Tag != nil {
		tag = c.VisitExpr(stmt.Tag)
	} else {
		tag = c.NewConstValue(exact.MakeBool(true), types.Typ[types.Bool])
	}
	if len(stmt.Body.List) == 0 {
		return
	}

	// Convert untyped constant clauses.
	for _, clause := range stmt.Body.List {
		for _, expr := range clause.(*ast.CaseClause).List {
			if typ := c.typeinfo.Types[expr]; isUntyped(typ) {
				c.typeinfo.Types[expr] = tag.Type()
			}
		}
	}

	// makeValueFunc takes an expression, evaluates it, and returns
	// a Value representing its equality comparison with the tag.
	makeValueFunc := func(expr ast.Expr) func() Value {
		return func() Value {
			return c.VisitExpr(expr).BinaryOp(token.EQL, tag)
		}
	}

	// Create a BasicBlock for each case clause and each associated
	// statement body. Each case clause will branch to either its
	// statement body (success) or to the next case (failure), or the
	// end block if there are no remaining cases.
	startBlock := c.builder.GetInsertBlock()
	endBlock := llvm.AddBasicBlock(startBlock.Parent(), "end")
	endBlock.MoveAfter(startBlock)
	defer c.builder.SetInsertPointAtEnd(endBlock)

	if c.lastlabel != nil {
		labelData := c.labelData(c.lastlabel)
		labelData.Break = endBlock
		c.lastlabel = nil
	}

	// Add a "break" block to the stack.
	c.breakblocks = append(c.breakblocks, endBlock)
	defer func() { c.breakblocks = c.breakblocks[:len(c.breakblocks)-1] }()

	caseBlocks := make([]llvm.BasicBlock, 0, len(stmt.Body.List))
	stmtBlocks := make([]llvm.BasicBlock, 0, len(stmt.Body.List))
	for _ = range stmt.Body.List {
		caseBlocks = append(caseBlocks, llvm.InsertBasicBlock(endBlock, ""))
	}
	for _ = range stmt.Body.List {
		stmtBlocks = append(stmtBlocks, llvm.InsertBasicBlock(endBlock, ""))
	}

	// Move the "default" block to the end, if there is one.
	caseclauses := make([]*ast.CaseClause, 0, len(stmt.Body.List))
	var defaultclause *ast.CaseClause
	for _, stmt := range stmt.Body.List {
		clause := stmt.(*ast.CaseClause)
		if clause.List == nil {
			defaultclause = clause
		} else {
			caseclauses = append(caseclauses, clause)
		}
	}
	if defaultclause != nil {
		caseclauses = append(caseclauses, defaultclause)
	}

	c.builder.CreateBr(caseBlocks[0])
	for i, clause := range caseclauses {
		c.builder.SetInsertPointAtEnd(caseBlocks[i])
		stmtBlock := stmtBlocks[i]
		nextBlock := endBlock
		if i+1 < len(caseBlocks) {
			nextBlock = caseBlocks[i+1]
		}

		if clause.List != nil {
			value := c.VisitExpr(clause.List[0])
			result := value.BinaryOp(token.EQL, tag)
			for _, expr := range clause.List[1:] {
				rhsResultFunc := makeValueFunc(expr)
				result = c.compileLogicalOp(token.LOR, result, rhsResultFunc)
			}
			c.builder.CreateCondBr(result.LLVMValue(), stmtBlock, nextBlock)
		} else {
			// default case
			c.builder.CreateBr(stmtBlock)
		}

		c.builder.SetInsertPointAtEnd(stmtBlock)
		branchBlock := endBlock
		for _, stmt := range clause.Body {
			if br, isbr := stmt.(*ast.BranchStmt); isbr {
				if br.Tok == token.FALLTHROUGH {
					if i+1 < len(stmtBlocks) {
						branchBlock = stmtBlocks[i+1]
					}
				} else {
					c.VisitStmt(stmt)
				}
				// Ignore anything after a branch statement.
				break
			} else {
				c.VisitStmt(stmt)
			}
		}
		c.maybeImplicitBranch(branchBlock)
	}
}

func (c *compiler) VisitRangeStmt(stmt *ast.RangeStmt) {
	currBlock := c.builder.GetInsertBlock()
	doneBlock := llvm.AddBasicBlock(currBlock.Parent(), "done")
	doneBlock.MoveAfter(currBlock)
	postBlock := llvm.InsertBasicBlock(doneBlock, "post")
	loopBlock := llvm.InsertBasicBlock(postBlock, "loop")
	condBlock := llvm.InsertBasicBlock(loopBlock, "cond")
	defer c.builder.SetInsertPointAtEnd(doneBlock)

	// Evaluate range expression first.
	x := c.VisitExpr(stmt.X)

	// If it's a pointer type, we'll first check that it's non-nil.
	typ := x.Type().Underlying()
	if _, ok := typ.(*types.Pointer); ok {
		ifBlock := llvm.InsertBasicBlock(doneBlock, "if")
		isnotnull := c.builder.CreateIsNotNull(x.LLVMValue(), "")
		c.builder.CreateCondBr(isnotnull, ifBlock, doneBlock)
		c.builder.SetInsertPointAtEnd(ifBlock)
	}

	// Is it a new var definition? Then allocate some memory on the stack.
	var keyPtr, valuePtr llvm.Value
	if stmt.Tok == token.DEFINE {
		if key := stmt.Key.(*ast.Ident); !isBlank(key.Name) {
			keyobj := c.typeinfo.Objects[key]
			keyType := keyobj.Type()
			keyPtr = c.builder.CreateAlloca(c.types.ToLLVM(keyType), "")
			stackvar := c.NewValue(keyPtr, types.NewPointer(keyType)).makePointee()
			stackvar.stack = c.functions.top().LLVMValue
			c.objectdata[keyobj].Value = stackvar
		}
		if stmt.Value != nil {
			if value := stmt.Value.(*ast.Ident); !isBlank(value.Name) {
				valueobj := c.typeinfo.Objects[value]
				valueType := valueobj.Type()
				valuePtr = c.builder.CreateAlloca(c.types.ToLLVM(valueType), "")
				stackvar := c.NewValue(valuePtr, types.NewPointer(valueType)).makePointee()
				stackvar.stack = c.functions.top().LLVMValue
				c.objectdata[valueobj].Value = stackvar
			}
		}
	} else {
		// Simple assignment, resolve the key/value pointers.
		if key := stmt.Key.(*ast.Ident); !isBlank(key.Name) {
			keyPtr = c.Resolve(key).(*LLVMValue).pointer.LLVMValue()
		}
		if stmt.Value != nil {
			if value := stmt.Value.(*ast.Ident); !isBlank(value.Name) {
				valuePtr = c.Resolve(value).(*LLVMValue).pointer.LLVMValue()
			}
		}
	}

	if c.lastlabel != nil {
		labelData := c.labelData(c.lastlabel)
		labelData.Break = doneBlock
		labelData.Continue = postBlock
		c.lastlabel = nil
	}

	c.breakblocks = append(c.breakblocks, doneBlock)
	c.continueblocks = append(c.continueblocks, postBlock)
	defer func() {
		c.breakblocks = c.breakblocks[:len(c.breakblocks)-1]
		c.continueblocks = c.continueblocks[:len(c.continueblocks)-1]
	}()

	isarray := false
	var base, length llvm.Value
	_, isptr := typ.(*types.Pointer)
	if isptr {
		typ = typ.(*types.Pointer).Elem()
	}
	switch typ := typ.Underlying().(type) {
	case *types.Map:
		goto maprange
	case *types.Basic:
		stringvalue := x.LLVMValue()
		length = c.builder.CreateExtractValue(stringvalue, 1, "")
		goto stringrange
	case *types.Array:
		isarray = true
		x := x
		if !isptr {
			if x_, ok := x.(*LLVMValue); ok && x_.pointer != nil {
				x = x_.pointer
			} else {
				// TODO load value onto stack for indexing?
			}
		}
		base = x.LLVMValue()
		length = llvm.ConstInt(c.llvmtypes.inttype, uint64(typ.Len()), false)
		goto arrayrange
	case *types.Slice:
		slicevalue := x.LLVMValue()
		base = c.builder.CreateExtractValue(slicevalue, 0, "")
		length = c.builder.CreateExtractValue(slicevalue, 1, "")
		goto arrayrange
	}
	panic("unreachable")

maprange:
	{
		currBlock = c.builder.GetInsertBlock()
		c.builder.CreateBr(condBlock)
		c.builder.SetInsertPointAtEnd(condBlock)
		nextptrphi := c.builder.CreatePHI(c.target.IntPtrType(), "next")
		nextptr, pk, pv := c.mapNext(x.(*LLVMValue), nextptrphi)
		notnull := c.builder.CreateIsNotNull(nextptr, "")
		c.builder.CreateCondBr(notnull, loopBlock, doneBlock)
		c.builder.SetInsertPointAtEnd(loopBlock)
		if !keyPtr.IsNil() {
			keyval := c.builder.CreateLoad(pk, "")
			c.builder.CreateStore(keyval, keyPtr)
		}
		if !valuePtr.IsNil() {
			valval := c.builder.CreateLoad(pv, "")
			c.builder.CreateStore(valval, valuePtr)
		}
		c.VisitBlockStmt(stmt.Body, false)
		c.maybeImplicitBranch(postBlock)
		c.builder.SetInsertPointAtEnd(postBlock)
		c.builder.CreateBr(condBlock)
		nextptrphi.AddIncoming([]llvm.Value{llvm.ConstNull(c.target.IntPtrType()), nextptr}, []llvm.BasicBlock{currBlock, postBlock})
		return
	}

stringrange:
	{
		zero := llvm.ConstNull(c.types.inttype)
		currBlock = c.builder.GetInsertBlock()
		c.builder.CreateBr(condBlock)
		c.builder.SetInsertPointAtEnd(condBlock)
		index := c.builder.CreatePHI(c.types.inttype, "index")
		lessthan := c.builder.CreateICmp(llvm.IntULT, index, length, "")
		c.builder.CreateCondBr(lessthan, loopBlock, doneBlock)
		c.builder.SetInsertPointAtEnd(loopBlock)
		consumed, value := c.stringNext(x.LLVMValue(), index)
		if !keyPtr.IsNil() {
			c.builder.CreateStore(index, keyPtr)
		}
		if !valuePtr.IsNil() {
			c.builder.CreateStore(value, valuePtr)
		}
		c.VisitBlockStmt(stmt.Body, false)
		c.maybeImplicitBranch(postBlock)
		c.builder.SetInsertPointAtEnd(postBlock)
		newindex := c.builder.CreateAdd(index, consumed, "")
		c.builder.CreateBr(condBlock)
		index.AddIncoming([]llvm.Value{zero, newindex}, []llvm.BasicBlock{currBlock, postBlock})
		return
	}

arrayrange:
	{
		zero := llvm.ConstNull(c.types.inttype)
		currBlock = c.builder.GetInsertBlock()
		c.builder.CreateBr(condBlock)
		c.builder.SetInsertPointAtEnd(condBlock)
		index := c.builder.CreatePHI(c.types.inttype, "index")
		lessthan := c.builder.CreateICmp(llvm.IntULT, index, length, "")
		c.builder.CreateCondBr(lessthan, loopBlock, doneBlock)
		c.builder.SetInsertPointAtEnd(loopBlock)
		if !keyPtr.IsNil() {
			c.builder.CreateStore(index, keyPtr)
		}
		if !valuePtr.IsNil() {
			var indices []llvm.Value
			if isarray {
				indices = []llvm.Value{zero, index}
			} else {
				indices = []llvm.Value{index}
			}
			elementptr := c.builder.CreateGEP(base, indices, "")
			element := c.builder.CreateLoad(elementptr, "")
			c.builder.CreateStore(element, valuePtr)
		}
		c.VisitBlockStmt(stmt.Body, false)
		c.maybeImplicitBranch(postBlock)
		c.builder.SetInsertPointAtEnd(postBlock)
		newindex := c.builder.CreateAdd(index, llvm.ConstInt(c.types.inttype, 1, false), "")
		c.builder.CreateBr(condBlock)
		index.AddIncoming([]llvm.Value{zero, newindex}, []llvm.BasicBlock{currBlock, postBlock})
		return
	}
}

func (c *compiler) VisitBranchStmt(stmt *ast.BranchStmt) {
	switch stmt.Tok {
	case token.BREAK:
		var block llvm.BasicBlock
		if stmt.Label == nil {
			block = c.breakblocks[len(c.breakblocks)-1]
		} else {
			block = c.labelData(stmt.Label).Break
		}
		c.builder.CreateBr(block)
	case token.CONTINUE:
		var block llvm.BasicBlock
		if stmt.Label == nil {
			block = c.continueblocks[len(c.continueblocks)-1]
		} else {
			block = c.labelData(stmt.Label).Continue
		}
		c.builder.CreateBr(block)
	case token.GOTO:
		labelData := c.labelData(stmt.Label)
		c.builder.CreateBr(labelData.Goto)
	default:
		panic("unimplemented: " + stmt.Tok.String())
	}
}

func (c *compiler) VisitTypeSwitchStmt(stmt *ast.TypeSwitchStmt) {
	if stmt.Init != nil {
		c.VisitStmt(stmt.Init)
	}

	var assignIdent *ast.Ident
	var typeAssertExpr *ast.TypeAssertExpr
	switch stmt := stmt.Assign.(type) {
	case *ast.AssignStmt:
		assignIdent = stmt.Lhs[0].(*ast.Ident)
		typeAssertExpr = stmt.Rhs[0].(*ast.TypeAssertExpr)
	case *ast.ExprStmt:
		typeAssertExpr = stmt.X.(*ast.TypeAssertExpr)
	}
	if len(stmt.Body.List) == 0 {
		// No case clauses, so just evaluate the expression.
		c.VisitExpr(typeAssertExpr.X)
		return
	}

	currBlock := c.builder.GetInsertBlock()
	endBlock := llvm.AddBasicBlock(currBlock.Parent(), "")
	endBlock.MoveAfter(currBlock)
	defer c.builder.SetInsertPointAtEnd(endBlock)

	// Add a "break" block to the stack.
	c.breakblocks = append(c.breakblocks, endBlock)
	defer func() { c.breakblocks = c.breakblocks[:len(c.breakblocks)-1] }()

	// TODO: investigate the use of a switch instruction
	//       on the type's hash (when we compute type hashes).

	// Create blocks for each statement.
	defaultBlock := endBlock
	var condBlocks []llvm.BasicBlock
	var stmtBlocks []llvm.BasicBlock
	for _, stmt := range stmt.Body.List {
		caseClause := stmt.(*ast.CaseClause)
		if caseClause.List == nil {
			defaultBlock = llvm.InsertBasicBlock(endBlock, "")
		} else {
			condBlock := llvm.InsertBasicBlock(endBlock, "")
			stmtBlock := llvm.InsertBasicBlock(endBlock, "")
			condBlocks = append(condBlocks, condBlock)
			stmtBlocks = append(stmtBlocks, stmtBlock)
		}
	}
	stmtBlocks = append(stmtBlocks, defaultBlock)

	// Evaluate the expression, then jump to the first condition block.
	iface := c.VisitExpr(typeAssertExpr.X).(*LLVMValue)
	if len(stmt.Body.List) == 1 && defaultBlock != endBlock {
		c.builder.CreateBr(defaultBlock)
	} else {
		c.builder.CreateBr(condBlocks[0])
	}

	i := 0
	for _, stmt := range stmt.Body.List {
		caseClause := stmt.(*ast.CaseClause)
		if caseClause.List != nil {
			c.builder.SetInsertPointAtEnd(condBlocks[i])
			stmtBlock := stmtBlocks[i]
			nextCondBlock := defaultBlock
			if i+1 < len(condBlocks) {
				nextCondBlock = condBlocks[i+1]
			}
			caseCond := func(j int) Value {
				if c.isNilIdent(caseClause.List[j]) {
					iface := iface.LLVMValue()
					ifacetyp := c.builder.CreateExtractValue(iface, 0, "")
					isnil := c.builder.CreateIsNull(ifacetyp, "")
					return c.NewValue(isnil, types.Typ[types.Bool])
				}
				typ := c.typeinfo.Types[caseClause.List[j]]
				switch typ := typ.Underlying().(type) {
				case *types.Interface:
					_, ok := iface.convertI2I(typ)
					return ok
				}
				return iface.interfaceTypeEquals(typ)
			}
			cond := caseCond(0)
			for j := 1; j < len(caseClause.List); j++ {
				f := func() Value {
					return caseCond(j)
				}
				cond = c.compileLogicalOp(token.LOR, cond, f).(*LLVMValue)
			}
			c.builder.CreateCondBr(cond.LLVMValue(), stmtBlock, nextCondBlock)
			i++
		}
	}

	i = 0
	for _, stmt := range stmt.Body.List {
		caseClause := stmt.(*ast.CaseClause)
		var block llvm.BasicBlock
		var typ types.Type
		if caseClause.List != nil {
			block = stmtBlocks[i]
			if len(caseClause.List) == 1 {
				typ = c.typeinfo.Types[caseClause.List[0]]
			}
			i++
		} else {
			block = defaultBlock
		}
		c.builder.SetInsertPointAtEnd(block)
		if assignIdent != nil {
			obj := c.typeinfo.Implicits[caseClause]
			if len(caseClause.List) == 1 && !c.isNilIdent(caseClause.List[0]) {
				switch utyp := typ.Underlying().(type) {
				case *types.Interface:
					// FIXME Use value from convertI2I in the case
					// clause condition test.
					c.objectdata[obj].Value, _ = iface.convertI2I(utyp)
				default:
					c.objectdata[obj].Value = iface.loadI2V(typ)
				}
			} else {
				c.objectdata[obj].Value = iface
			}
		}
		for _, stmt := range caseClause.Body {
			c.VisitStmt(stmt)
		}
		c.maybeImplicitBranch(endBlock)
	}
}

func (c *compiler) VisitSelectStmt(stmt *ast.SelectStmt) {
	// TODO
	/*
		if c.lastlabel != nil {
			labelData := c.labelData(c.lastlabel)
			labelData.Break = doneBlock
			c.lastlabel = nil
		}
	*/
}

func (c *compiler) VisitStmt(stmt ast.Stmt) {
	if c.Logger != nil {
		c.Logger.Println("Compile statement:", reflect.TypeOf(stmt),
			"@", c.fileset.Position(stmt.Pos()))
	}
	switch x := stmt.(type) {
	case *ast.ReturnStmt:
		c.VisitReturnStmt(x)
	case *ast.AssignStmt:
		c.VisitAssignStmt(x)
	case *ast.IncDecStmt:
		c.VisitIncDecStmt(x)
	case *ast.IfStmt:
		c.VisitIfStmt(x)
	case *ast.ForStmt:
		c.VisitForStmt(x)
	case *ast.ExprStmt:
		c.VisitExpr(x.X)
	case *ast.BlockStmt:
		c.VisitBlockStmt(x, true)
	case *ast.DeclStmt:
		c.VisitDecl(x.Decl)
	case *ast.GoStmt:
		c.VisitGoStmt(x)
	case *ast.SwitchStmt:
		c.VisitSwitchStmt(x)
	case *ast.RangeStmt:
		c.VisitRangeStmt(x)
	case *ast.BranchStmt:
		c.VisitBranchStmt(x)
	case *ast.TypeSwitchStmt:
		c.VisitTypeSwitchStmt(x)
	case *ast.LabeledStmt:
		c.VisitLabeledStmt(x)
	case *ast.DeferStmt:
		c.VisitDeferStmt(x)
	case *ast.SendStmt:
		c.VisitSendStmt(x)
	case *ast.SelectStmt:
		c.VisitSelectStmt(x)
	default:
		panic(fmt.Sprintf("Unhandled Stmt node: %s", reflect.TypeOf(stmt)))
	}
}

// vim: set ft=go :
