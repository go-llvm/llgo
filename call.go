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
func (c *compiler) createCall(fn *LLVMValue, argValues []*LLVMValue) []*LLVMValue {
	fntyp := fn.Type().Underlying().(*types.Signature)
	typinfo := c.types.getSignatureInfo(fntyp)

	args := make([]llvm.Value, len(argValues))
	for i, arg := range argValues {
		args[i] = arg.LLVMValue()
	}
	results := typinfo.call(c.types.ctx, c.allocaBuilder, c.builder, fn.LLVMValue(), args)

	resultValues := make([]*LLVMValue, len(results))
	for i, res := range results {
		resultValues[i] = c.NewValue(res, fntyp.Results().At(i).Type())
	}
	return resultValues
}
