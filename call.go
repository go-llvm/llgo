// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// createCall emits the code for a function call, taking into account
// variadic functions, receivers, and panic/defer.
//
// dotdotdot is true if the last argument is followed with "...".
func (c *compiler) createCall(fn *LLVMValue, argValues []*LLVMValue, dotdotdot, invoke bool) *LLVMValue {
	fn_type := fn.Type().Underlying().(*types.Signature)
	var args []llvm.Value

	// TODO Move all of this to evalCallArgs?
	params := fn_type.Params()
	if nparams := int(params.Len()); nparams > 0 {
		if fn_type.IsVariadic() {
			nparams--
		}
		for i := 0; i < nparams; i++ {
			value := argValues[i]
			args = append(args, value.LLVMValue())
		}
		if fn_type.IsVariadic() {
			if dotdotdot {
				// Calling f(x...). Just pass the slice directly.
				slice_value := argValues[nparams].LLVMValue()
				args = append(args, slice_value)
			} else {
				varargs := make([]llvm.Value, len(argValues)-nparams)
				for i, value := range argValues[nparams:] {
					varargs[i] = value.LLVMValue()
				}
				param_type := params.At(nparams).Type().(*types.Slice).Elem()
				slice_value := c.makeLiteralSlice(varargs, param_type)
				args = append(args, slice_value)
			}
		}
	}

	var result_type types.Type
	switch results := fn_type.Results(); results.Len() {
	case 0: // no-op
	case 1:
		result_type = results.At(0).Type()
	default:
		result_type = results
	}

	// Depending on whether the function contains defer statements or not,
	// we'll generate either a "call" or an "invoke" instruction.
	var createCall = c.builder.CreateCall
	/*
		if invoke {
			f := c.functions.top()
			// TODO Create a method on compiler (avoid creating closures).
			createCall = func(fn llvm.Value, args []llvm.Value, name string) llvm.Value {
				currblock := c.builder.GetInsertBlock()
				returnblock := llvm.AddBasicBlock(currblock.Parent(), "")
				returnblock.MoveAfter(currblock)
				value := c.builder.CreateInvoke(fn, args, returnblock, f.unwindblock, "")
				c.builder.SetInsertPointAtEnd(returnblock)
				return value
			}
		}
	*/

	var fnptr llvm.Value
	fnval := fn.LLVMValue()
	if fnval.Type().TypeKind() == llvm.PointerTypeKind {
		fnptr = fnval
	} else {
		fnptr = c.builder.CreateExtractValue(fnval, 0, "")
		context := c.builder.CreateExtractValue(fnval, 1, "")
		fntyp := fnptr.Type().ElementType()
		paramTypes := fntyp.ParamTypes()

		// If the context is not a constant null, and we're not
		// dealing with a method (where we don't care about the value
		// of the receiver), then we must conditionally call the
		// function with the additional receiver/closure.
		if !context.IsNull() || fn_type.Recv() != nil {
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
				nullctxresult = createCall(fnptr, args, "")
				nullctxblock = c.builder.GetInsertBlock()
				c.builder.CreateBr(endblock)
				c.builder.SetInsertPointAtEnd(nonnullctxblock)
			}

			// non-null context case.
			var result llvm.Value
			args := append([]llvm.Value{context}, args...)
			if len(paramTypes) < len(args) {
				returnType := fntyp.ReturnType()
				ctxType := context.Type()
				paramTypes := append([]llvm.Type{ctxType}, paramTypes...)
				vararg := fntyp.IsFunctionVarArg()
				fntyp := llvm.FunctionType(returnType, paramTypes, vararg)
				fnptrtyp := llvm.PointerType(fntyp, 0)
				fnptr = c.builder.CreateBitCast(fnptr, fnptrtyp, "")
			}
			result = createCall(fnptr, args, "")

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
			return c.NewValue(result, result_type)
		}
	}
	result := createCall(fnptr, args, "")
	return c.NewValue(result, result_type)
}
