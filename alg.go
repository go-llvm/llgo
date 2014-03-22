// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"

	"github.com/axw/gollvm/llvm"
)

type AlgorithmKind int

const (
	AlgorithmHash AlgorithmKind = iota
	AlgorithmEqual
	AlgorithmPrint
	AlgorithmCopy
)

// algorithmMap maps types to their runtime algorithms (equality, hash, etc.)
type algorithmMap struct {
	module  llvm.Module
	runtime *runtimeInterface

	hashAlgFunctionType,
	equalAlgFunctionType,
	printAlgFunctionType,
	copyAlgFunctionType llvm.Type
}

func newAlgorithmMap(m llvm.Module, runtime *runtimeInterface, target llvm.TargetData) *algorithmMap {
	am := &algorithmMap{
		module:  m,
		runtime: runtime,
	}
	uintptrType := target.IntPtrType()
	voidPtrType := llvm.PointerType(llvm.Int8Type(), 0)
	boolType := llvm.Int8Type()
	params := []llvm.Type{uintptrType, voidPtrType}
	am.hashAlgFunctionType = llvm.FunctionType(uintptrType, params, false)
	params = []llvm.Type{uintptrType, uintptrType, uintptrType}
	am.equalAlgFunctionType = llvm.FunctionType(boolType, params, false)
	params = []llvm.Type{uintptrType, voidPtrType}
	am.printAlgFunctionType = llvm.FunctionType(llvm.VoidType(), params, false)
	params = []llvm.Type{uintptrType, voidPtrType, voidPtrType}
	am.copyAlgFunctionType = llvm.FunctionType(llvm.VoidType(), params, false)
	return am
}

func (am *algorithmMap) eqalg(t types.Type) llvm.Value {
	t = t.Underlying()
	if st, ok := t.(*types.Struct); ok && st.NumFields() == 1 {
		t = st.Field(0).Type().Underlying()
	}
	switch t := t.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.String:
			return am.runtime.streqalg.LLVMValue()
		case types.Float32:
			return am.runtime.f32eqalg.LLVMValue()
		case types.Float64:
			return am.runtime.f64eqalg.LLVMValue()
		case types.Complex64:
			return am.runtime.c64eqalg.LLVMValue()
		case types.Complex128:
			return am.runtime.c128eqalg.LLVMValue()
		}
	case *types.Struct:
		// TODO
	}
	// TODO(axw) size-specific memequal cases
	return am.runtime.memequal.LLVMValue()
}
