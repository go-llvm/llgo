// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// createCall emits the code for a function call,
// taking into account receivers, and panic/defer.
//
// dotdotdot is true if the last argument is followed with "...".
func (c *compiler) createCall(fn *LLVMValue, argValues []*LLVMValue, invoke bool) *LLVMValue {
	fntyp := fn.Type().Underlying().(*types.Signature)
	args := make([]llvm.Value, len(argValues))
	for i, arg := range argValues {
		args[i] = arg.LLVMValue()
	}

	var resultType types.Type
	switch results := fntyp.Results(); results.Len() {
	case 0: // no-op
	case 1:
		resultType = results.At(0).Type()
	default:
		resultType = results
	}

	var fnptr llvm.Value
	fnval := fn.LLVMValue()
	if fnval.Type().TypeKind() == llvm.PointerTypeKind {
		fnptr = fnval
	} else {
		fnptr = c.builder.CreateExtractValue(fnval, 0, "")
		context := c.builder.CreateExtractValue(fnval, 1, "")
		llfntyp := fnptr.Type().ElementType()
		paramTypes := llfntyp.ParamTypes()

		// If the context is not a constant null, and we're not
		// dealing with a method (where we don't care about the value
		// of the receiver), then we must conditionally call the
		// function with the additional receiver/closure.
		if !context.IsNull() && fntyp.Recv() == nil {
			// Store the blocks for referencing in the Phi below;
			// note that we update the block after each createCall,
			// since createCall may create new blocks and we want
			// the predecessors to the Phi.
			var nullctxblock llvm.BasicBlock
			var nonnullctxblock llvm.BasicBlock
			var endblock llvm.BasicBlock
			var nullctxresult llvm.Value

			// len(paramTypes) == len(args) iff function is not a method.
			if !context.IsConstant() && len(paramTypes) == len(args) {
				currblock := c.builder.GetInsertBlock()
				endblock = llvm.AddBasicBlock(currblock.Parent(), "")
				endblock.MoveAfter(currblock)
				nonnullctxblock = llvm.InsertBasicBlock(endblock, "")
				nullctxblock = llvm.InsertBasicBlock(nonnullctxblock, "")
				nullctx := c.builder.CreateIsNull(context, "")
				c.builder.CreateCondBr(nullctx, nullctxblock, nonnullctxblock)

				// null context case.
				c.builder.SetInsertPointAtEnd(nullctxblock)
				nullctxresult = c.builder.CreateCall(fnptr, args, "")
				nullctxblock = c.builder.GetInsertBlock()
				c.builder.CreateBr(endblock)
				c.builder.SetInsertPointAtEnd(nonnullctxblock)
			}

			// non-null context case.
			var result llvm.Value
			args := append([]llvm.Value{context}, args...)
			if len(paramTypes) < len(args) {
				returnType := llfntyp.ReturnType()
				ctxType := context.Type()
				paramTypes := append([]llvm.Type{ctxType}, paramTypes...)
				vararg := llfntyp.IsFunctionVarArg()
				llfntyp := llvm.FunctionType(returnType, paramTypes, vararg)
				fnptrtyp := llvm.PointerType(llfntyp, 0)
				fnptr = c.builder.CreateBitCast(fnptr, fnptrtyp, "")
			}
			result = c.builder.CreateCall(fnptr, args, "")

			// If the return type is not void, create a
			// PHI node to select which value to return.
			if !nullctxresult.IsNil() {
				nonnullctxblock = c.builder.GetInsertBlock()
				c.builder.CreateBr(endblock)
				c.builder.SetInsertPointAtEnd(endblock)
				if result.Type().TypeKind() != llvm.VoidTypeKind {
					phiresult := c.builder.CreatePHI(result.Type(), "")
					values := []llvm.Value{nullctxresult, result}
					blocks := []llvm.BasicBlock{nullctxblock, nonnullctxblock}
					phiresult.AddIncoming(values, blocks)
					result = phiresult
				}
			}
			return c.NewValue(result, resultType)
		}
	}
	result := c.builder.CreateCall(fnptr, args, "")
	return c.NewValue(result, resultType)
}
