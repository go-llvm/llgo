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
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo/types"
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
	ptr := c.VisitExpr(stmt.X).(*LLVMValue).pointer
	value := c.builder.CreateLoad(ptr.LLVMValue(), "")
	one := llvm.ConstInt(value.Type(), 1, false)

	switch stmt.Tok {
	case token.INC:
		value = c.builder.CreateAdd(value, one, "")
	case token.DEC:
		value = c.builder.CreateSub(value, one, "")
	}

	// TODO make sure we cover all possibilities (maybe just delegate this to
	// an assignment statement handler, and do it all in one place).
	//
	// In the case of a simple variable, we simply calculate the new value and
	// update the value in the scope.
	c.builder.CreateStore(value, ptr.LLVMValue())
}

func (c *compiler) VisitBlockStmt(stmt *ast.BlockStmt) {
	c.PushScope()
	defer c.PopScope()
	for _, stmt := range stmt.List {
		c.VisitStmt(stmt)
	}
}

func (c *compiler) VisitReturnStmt(stmt *ast.ReturnStmt) {
	f := c.functions[len(c.functions)-1]
	ftyp := f.Type().(*types.Func)
	if len(ftyp.Results) == 0 {
		c.builder.CreateRetVoid()
		return
	}

	values := make([]llvm.Value, len(ftyp.Results))
	if stmt.Results == nil {
		for i, obj := range ftyp.Results {
			values[i] = obj.Data.(*LLVMValue).LLVMValue()
		}
	} else {
		for i, expr := range stmt.Results {
			resultobj := ftyp.Results[i]
			value := c.VisitExpr(expr).Convert(resultobj.Type.(types.Type))
			values[i] = value.LLVMValue()
			if resultobj.Name != "_" && resultobj.Name != "" {
				resultptr := resultobj.Data.(*LLVMValue).pointer.LLVMValue()
				c.builder.CreateStore(value.LLVMValue(), resultptr)
			}
		}
	}

	if len(values) == 1 {
		c.builder.CreateRet(values[0])
	} else {
		c.builder.CreateAggregateRet(values)
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

func (c *compiler) VisitAssignStmt(stmt *ast.AssignStmt) {
	// x (add_op|mul_op)= y
	if stmt.Tok != token.DEFINE && stmt.Tok != token.ASSIGN {
		// TODO handle assignment to map element.
		op := nonAssignmentToken(stmt.Tok)
		lhs := c.VisitExpr(stmt.Lhs[0])
		rhsValue := c.VisitExpr(stmt.Rhs[0])
		newValue := lhs.BinaryOp(op, rhsValue).(*LLVMValue).LLVMValue()
		c.builder.CreateStore(newValue, lhs.(*LLVMValue).pointer.LLVMValue())
		return
	}

	// a, b, ... [:]= x, y, ...
	values := make([]Value, len(stmt.Lhs))
	if len(stmt.Rhs) == 1 && len(stmt.Lhs) > 1 {
		value := c.VisitExpr(stmt.Rhs[0])
		ptr := value.LLVMValue()
		struct_type := value.Type().(*types.Struct)
		for i := 0; i < len(struct_type.Fields); i++ {
			t := c.ObjGetType(struct_type.Fields[i])
			value_ := c.builder.CreateExtractValue(ptr, i, "")
			values[i] = c.NewLLVMValue(value_, t)
		}
	} else {
		for i, expr := range stmt.Rhs {
			values[i] = c.VisitExpr(expr)
		}
	}
	for i, expr := range stmt.Lhs {
		value := values[i]
		switch x := expr.(type) {
		case *ast.Ident:
			if x.Name != "_" {
				obj := x.Obj
				if stmt.Tok == token.DEFINE {
					value_type := value.LLVMValue().Type()
					ptr := c.builder.CreateAlloca(value_type, x.Name)
					c.builder.CreateStore(value.LLVMValue(), ptr)
					llvm_value := c.NewLLVMValue(
						ptr, &types.Pointer{Base: value.Type()})
					obj.Data = llvm_value.makePointee()
				} else {
					ptr := (obj.Data).(*LLVMValue).pointer
					value = value.Convert(types.Deref(ptr.Type()))
					c.builder.CreateStore(value.LLVMValue(), ptr.LLVMValue())
				}
			}
		case *ast.IndexExpr:
			if t, ok := c.types.expr[x.X]; ok {
				if _, ok := t.(*types.Map); ok {
					m := c.VisitExpr(x.X).(*LLVMValue)
					index := c.VisitExpr(x.Index)
					ptr := c.mapLookup(m, index, true).pointer
					value = value.Convert(types.Deref(ptr.Type()))
					c.builder.CreateStore(value.LLVMValue(), ptr.LLVMValue())
					return
				}
			}
		}
		// default (since we can't fallthrough in non-map index exprs)
		ptr := c.VisitExpr(expr).(*LLVMValue).pointer
		value = value.Convert(types.Deref(ptr.Type()))
		c.builder.CreateStore(value.LLVMValue(), ptr.LLVMValue())
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
		c.PushScope()
		c.VisitStmt(stmt.Init)
		defer c.PopScope()
	}

	cond_val := c.VisitExpr(stmt.Cond)
	c.builder.CreateCondBr(cond_val.LLVMValue(), ifBlock, elseBlock)
	c.builder.SetInsertPointAtEnd(ifBlock)
	c.VisitBlockStmt(stmt.Body)
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

	// Is there an initializer? Create a new scope and visit the statement.
	if stmt.Init != nil {
		c.PushScope()
		c.VisitStmt(stmt.Init)
		defer c.PopScope()
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
	c.VisitBlockStmt(stmt.Body)
	c.maybeImplicitBranch(postBlock)
}

func (c *compiler) VisitGoStmt(stmt *ast.GoStmt) {
	//stmt.Call *ast.CallExpr
	// TODO 
	var fn *LLVMValue
	switch x := (stmt.Call.Fun).(type) {
	case *ast.Ident:
		fn = c.Resolve(x.Obj).(*LLVMValue)
		if fn == nil {
			panic(fmt.Sprintf(
				"No function found with name '%s'", x.String()))
		}
	default:
		fn = c.VisitExpr(stmt.Call.Fun).(*LLVMValue)
	}

	// Evaluate arguments, store in a structure on the stack.
	var args_struct_type llvm.Type
	var args_mem llvm.Value
	var args_size llvm.Value
	if stmt.Call.Args != nil {
		param_types := make([]llvm.Type, 0)
		fn_type := types.Deref(fn.Type()).(*types.Func)
		for _, param := range fn_type.Params {
			typ := param.Type.(types.Type)
			param_types = append(param_types, c.types.ToLLVM(typ))
		}
		args_struct_type = llvm.StructType(param_types, false)
		args_mem = c.builder.CreateAlloca(args_struct_type, "")
		for i, expr := range stmt.Call.Args {
			value_i := c.VisitExpr(expr)
			value_i = value_i.Convert(fn_type.Params[i].Type.(types.Type))
			arg_i := c.builder.CreateGEP(args_mem, []llvm.Value{
				llvm.ConstInt(llvm.Int32Type(), 0, false),
				llvm.ConstInt(llvm.Int32Type(), uint64(i), false)}, "")
			c.builder.CreateStore(value_i.LLVMValue(), arg_i)
		}
		args_size = llvm.SizeOf(args_struct_type)
		args_size = llvm.ConstTrunc(args_size, llvm.Int32Type())
	} else {
		args_struct_type = llvm.VoidType()
		args_mem = llvm.ConstNull(llvm.PointerType(args_struct_type, 0))
		args_size = llvm.ConstInt(llvm.Int32Type(), 0, false)
	}

	// When done, return to where we were.
	defer c.builder.SetInsertPointAtEnd(c.builder.GetInsertBlock())

	// Create a function that will take a pointer to a structure of the type
	// defined above, or no parameters if there are none to pass.
	indirect_fn_type := llvm.FunctionType(
		llvm.VoidType(),
		[]llvm.Type{llvm.PointerType(args_struct_type, 0)}, false)
	indirect_fn := llvm.AddFunction(c.module.Module, "", indirect_fn_type)
	indirect_fn.SetFunctionCallConv(llvm.CCallConv)

	// Call "newgoroutine" with the indirect function and stored args.
	newgoroutine := getnewgoroutine(c.module.Module)
	ngr_param_types := newgoroutine.Type().ElementType().ParamTypes()
	fn_arg := c.builder.CreateBitCast(indirect_fn, ngr_param_types[0], "")
	args_arg := c.builder.CreateBitCast(args_mem,
		llvm.PointerType(llvm.Int8Type(), 0), "")
	c.builder.CreateCall(newgoroutine,
		[]llvm.Value{fn_arg, args_arg, args_size}, "")

	entry := llvm.AddBasicBlock(indirect_fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)
	var args []llvm.Value
	if stmt.Call.Args != nil {
		args_mem = indirect_fn.Param(0)
		args = make([]llvm.Value, len(stmt.Call.Args))
		for i := range stmt.Call.Args {
			arg_i := c.builder.CreateGEP(args_mem, []llvm.Value{
				llvm.ConstInt(llvm.Int32Type(), 0, false),
				llvm.ConstInt(llvm.Int32Type(), uint64(i), false)}, "")
			args[i] = c.builder.CreateLoad(arg_i, "")
		}
	}
	c.builder.CreateCall(fn.LLVMValue(), args, "")
	c.builder.CreateRetVoid()
}

func (c *compiler) VisitSwitchStmt(stmt *ast.SwitchStmt) {
	if stmt.Init != nil {
		c.PushScope()
		defer c.PopScope()
		c.VisitStmt(stmt.Init)
	}

	var tag Value
	if stmt.Tag != nil {
		tag = c.VisitExpr(stmt.Tag)
	} else {
		True := types.Universe.Lookup("true")
		tag = c.Resolve(True)
	}
	if len(stmt.Body.List) == 0 {
		return
	}

	// Create a BasicBlock for each case clause and each associated
	// statement body. Each case clause will branch to either its
	// statement body (success) or to the next case (failure), or the
	// end block if there are no remaining cases.
	startBlock := c.builder.GetInsertBlock()
	endBlock := llvm.AddBasicBlock(startBlock.Parent(), "end")
	endBlock.MoveAfter(startBlock)
	defer c.builder.SetInsertPointAtEnd(endBlock)

	caseBlocks := make([]llvm.BasicBlock, 0, len(stmt.Body.List))
	stmtBlocks := make([]llvm.BasicBlock, 0, len(stmt.Body.List))
	for _ = range stmt.Body.List {
		caseBlocks = append(caseBlocks, llvm.InsertBasicBlock(endBlock, ""))
	}
	for _ = range stmt.Body.List {
		stmtBlocks = append(stmtBlocks, llvm.InsertBasicBlock(endBlock, ""))
	}

	c.builder.CreateBr(caseBlocks[0])
	for i, stmt := range stmt.Body.List {
		c.builder.SetInsertPointAtEnd(caseBlocks[i])
		stmtBlock := stmtBlocks[i]
		nextBlock := endBlock
		if i+1 < len(caseBlocks) {
			nextBlock = caseBlocks[i+1]
		}

		clause := stmt.(*ast.CaseClause)
		if clause.List != nil {
			value := c.VisitExpr(clause.List[0])
			result := value.BinaryOp(token.EQL, tag)
			for _, expr := range clause.List[1:] {
				value = c.compileLogicalOp(token.LOR, result, expr)
			}
			c.builder.CreateCondBr(result.LLVMValue(), stmtBlock, nextBlock)
		} else {
			// default case
			c.builder.CreateBr(stmtBlock)
		}

		c.builder.SetInsertPointAtEnd(stmtBlock)
		branchBlock := endBlock
		for _, stmt := range clause.Body {
			if br, isbr := stmt.(*ast.BranchStmt); isbr && br.Tok == token.FALLTHROUGH {
				if i+1 < len(stmtBlocks) {
					branchBlock = stmtBlocks[i+1]
				}
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
	typ := types.Underlying(x.Type())
	if _, ok := typ.(*types.Pointer); ok {
		ifBlock := llvm.InsertBasicBlock(doneBlock, "if")
		isnotnull := c.builder.CreateIsNotNull(x.LLVMValue(), "")
		c.builder.CreateCondBr(isnotnull, ifBlock, doneBlock)
		c.builder.SetInsertPointAtEnd(ifBlock)
	}

	// Is it a new var definition? Then allocate some memory on the stack.
	var keyType, valueType types.Type
	var keyPtr, valuePtr llvm.Value
	if stmt.Tok == token.DEFINE {
		if key := stmt.Key.(*ast.Ident); key.Name != "_" {
			keyType = key.Obj.Type.(types.Type)
			keyPtr = c.builder.CreateAlloca(c.types.ToLLVM(keyType), "")
			key.Obj.Data = c.NewLLVMValue(keyPtr, &types.Pointer{Base: keyType}).makePointee()
		}
		if stmt.Value != nil {
			if value := stmt.Value.(*ast.Ident); value.Name != "_" {
				valueType = value.Obj.Type.(types.Type)
				valuePtr = c.builder.CreateAlloca(c.types.ToLLVM(valueType), "")
				value.Obj.Data = c.NewLLVMValue(valuePtr, &types.Pointer{Base: valueType}).makePointee()
			}
		}
	}

	isarray := false
	var base, length llvm.Value
	_, isptr := typ.(*types.Pointer)
	if isptr {
		typ = typ.(*types.Pointer).Base
	}
	switch typ := types.Underlying(typ).(type) {
	case *types.Map:
		// TODO
		panic("map range unimplemented")
	case *types.Name:
		// TODO
		panic("string range unimplemented")
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
		length = llvm.ConstInt(llvm.Int32Type(), typ.Len, false)
		goto arrayrange
	case *types.Slice:
		slicevalue := x.LLVMValue()
		base = c.builder.CreateExtractValue(slicevalue, 0, "")
		length = c.builder.CreateExtractValue(slicevalue, 1, "")
		goto arrayrange
	}

arrayrange:
	zero := llvm.ConstNull(llvm.Int32Type())
	currBlock = c.builder.GetInsertBlock()
	c.builder.CreateBr(condBlock)
	c.builder.SetInsertPointAtEnd(condBlock)
	index := c.builder.CreatePHI(llvm.Int32Type(), "index")
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
	c.VisitBlockStmt(stmt.Body)
	c.maybeImplicitBranch(postBlock)
	c.builder.SetInsertPointAtEnd(postBlock)
	newindex := c.builder.CreateAdd(index, llvm.ConstInt(llvm.Int32Type(), 1, false), "")
	c.builder.CreateBr(condBlock)
	index.AddIncoming([]llvm.Value{zero, newindex}, []llvm.BasicBlock{currBlock, postBlock})
}

func (c *compiler) VisitStmt(stmt ast.Stmt) {
	if c.logger != nil {
		c.logger.Println("Compile statement:", reflect.TypeOf(stmt),
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
		c.VisitBlockStmt(x)
	case *ast.DeclStmt:
		c.VisitDecl(x.Decl)
	case *ast.GoStmt:
		c.VisitGoStmt(x)
	case *ast.SwitchStmt:
		c.VisitSwitchStmt(x)
	case *ast.RangeStmt:
		c.VisitRangeStmt(x)
	default:
		panic(fmt.Sprintf("Unhandled Stmt node: %s", reflect.TypeOf(stmt)))
	}
}

// vim: set ft=go :
