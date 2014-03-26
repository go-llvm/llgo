// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/go-llvm/llvm"
)

// createCall emits the code for a function call,
// taking into account receivers, and panic/defer.
func (c *compiler) createCall(fn *LLVMValue, argValues []*LLVMValue) *LLVMValue {
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

	// Builtins are represented as a raw function pointer.
	fnval := fn.LLVMValue()
	if fnval.Type().TypeKind() == llvm.PointerTypeKind {
		return c.NewValue(c.builder.CreateCall(fnval, args, ""), resultType)
	}

	// If context is constant null, then the function does
	// not need a context argument.
	fnptr := c.builder.CreateExtractValue(fnval, 0, "")
	context := c.builder.CreateExtractValue(fnval, 1, "")
	llfntyp := fnptr.Type().ElementType()
	paramTypes := llfntyp.ParamTypes()
	if context.IsNull() {
		return c.NewValue(c.builder.CreateCall(fnptr, args, ""), resultType)
	}
	llfntyp = llvm.FunctionType(
		llfntyp.ReturnType(),
		append([]llvm.Type{context.Type()}, paramTypes...),
		llfntyp.IsFunctionVarArg(),
	)
	fnptr = c.builder.CreateBitCast(fnptr, llvm.PointerType(llfntyp, 0), "")
	result := c.builder.CreateCall(fnptr, append([]llvm.Value{context}, args...), "")
	return c.NewValue(result, resultType)
}
