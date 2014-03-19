// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/ssa"
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// createThunk creates a thunk from a
// given function and arguments, suitable for use with
// "defer" and "go".
func (fr *frame) createThunk(call *ssa.CallCommon) (thunk llvm.Value, arg llvm.Value) {
	var nonconstargs []llvm.Value
	var nonconstindices []int
	var nonconsttypes []*types.Var
	var args []llvm.Value
	packArg := func(arg *LLVMValue) {
		if arg.LLVMValue().IsAConstant().C != nil {
			args = append(args, arg.LLVMValue())
		} else {
			args = append(args, llvm.Value{nil})
			nonconstargs = append(nonconstargs, arg.LLVMValue())
			nonconstindices = append(nonconstindices, len(args)-1)
			nonconsttypes = append(nonconsttypes, types.NewField(0, nil, "", arg.Type(), true))
		}
	}

	_, isbuiltin := call.Value.(*ssa.Builtin)
	if !isbuiltin {
		packArg(fr.value(call.Value))
	}

	for _, arg := range call.Args {
		packArg(fr.value(arg))
	}

	i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
	if len(nonconstargs) == 0 {
		arg = llvm.ConstPointerNull(i8ptr)
	} else {
		structtype := types.NewStruct(nonconsttypes, nil)
		arg = fr.createTypeMalloc(structtype)
		for i, arg := range nonconstargs {
			argptr := fr.builder.CreateStructGEP(arg, i, "")
			fr.builder.CreateStore(arg, argptr)
		}
	}

	thunkfntype := llvm.FunctionType(llvm.VoidType(), []llvm.Type{i8ptr}, false)
	thunkfn := llvm.AddFunction(fr.module.Module, "", thunkfntype)
	thunkfn.SetLinkage(llvm.InternalLinkage)
	thunk = fr.builder.CreateBitCast(thunkfn, i8ptr, "")
	return
}
