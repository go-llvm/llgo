// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package irgen

import (
	"golang.org/x/tools/go/types"
	"llvm.org/llvm/bindings/go/llvm"
)

// createCall emits the code for a function call,
// taking into account receivers, and panic/defer.
func (fr *frame) createCall(fn *govalue, argValues []*govalue) []*govalue {
	fntyp := fn.Type().Underlying().(*types.Signature)
	typinfo := fr.types.getSignatureInfo(fntyp)

	args := make([]llvm.Value, len(argValues))
	for i, arg := range argValues {
		args[i] = arg.value
	}
	var results []llvm.Value
	if fr.unwindBlock.IsNil() {
		results = typinfo.call(fr.types.ctx, fr.allocaBuilder, fr.builder, fn.value, args)
	} else {
		contbb := llvm.AddBasicBlock(fr.function, "")
		results = typinfo.invoke(fr.types.ctx, fr.allocaBuilder, fr.builder, fn.value, args, contbb, fr.unwindBlock)
	}

	resultValues := make([]*govalue, len(results))
	for i, res := range results {
		resultValues[i] = newValue(res, fntyp.Results().At(i).Type())
	}
	return resultValues
}
