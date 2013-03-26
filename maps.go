// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.exp/go/types"
	"github.com/axw/gollvm/llvm"
)

// mapLookup searches a map for a specified key, returning a pointer to the
// memory location for the value. If insert is given as true, and the key
// does not exist in the map, it will be added with an uninitialised value.
func (c *compiler) mapLookup(m *LLVMValue, key Value, insert bool) (elem *LLVMValue, notnull *LLVMValue) {
	mapType := underlyingType(m.Type()).(*types.Map)
	maplookup := c.NamedFunction("runtime.maplookup", "func f(t, m, k uintptr, insert bool) uintptr")
	ptrType := c.target.IntPtrType()
	args := make([]llvm.Value, 4)
	args[0] = llvm.ConstPtrToInt(c.types.ToRuntime(mapType), ptrType)
	args[1] = c.builder.CreatePtrToInt(m.LLVMValue(), ptrType, "")
	if insert {
		args[3] = llvm.ConstAllOnes(llvm.Int1Type())
	} else {
		args[3] = llvm.ConstNull(llvm.Int1Type())
	}

	if lv, islv := key.(*LLVMValue); islv && lv.pointer != nil {
		args[2] = c.builder.CreatePtrToInt(lv.pointer.LLVMValue(), ptrType, "")
	}
	if args[2].IsNil() {
		stackval := c.builder.CreateAlloca(c.types.ToLLVM(key.Type()), "")
		c.builder.CreateStore(key.LLVMValue(), stackval)
		args[2] = c.builder.CreatePtrToInt(stackval, ptrType, "")
	}

	eltPtrType := &types.Pointer{Base: mapType.Elt}
	llvmtyp := c.types.ToLLVM(eltPtrType)
	zeroglobal := llvm.AddGlobal(c.module.Module, llvmtyp.ElementType(), "")
	zeroglobal.SetInitializer(llvm.ConstNull(llvmtyp.ElementType()))
	result := c.builder.CreateCall(maplookup, args, "")
	result = c.builder.CreateIntToPtr(result, llvmtyp, "")
	notnull_ := c.builder.CreateIsNotNull(result, "")
	result = c.builder.CreateSelect(notnull_, result, zeroglobal, "")
	value := c.NewValue(result, eltPtrType)
	return value.makePointee(), c.NewValue(notnull_, types.Typ[types.Bool])
}

func (c *compiler) mapDelete(m *LLVMValue, key Value) {
	mapdelete := c.NamedFunction("runtime.mapdelete", "func f(t, m, k uintptr)")
	mapType := underlyingType(m.Type()).(*types.Map)
	ptrType := c.target.IntPtrType()
	args := make([]llvm.Value, 3)
	args[0] = llvm.ConstPtrToInt(c.types.ToRuntime(mapType), ptrType)
	args[1] = c.builder.CreatePtrToInt(m.LLVMValue(), ptrType, "")
	if lv, islv := key.(*LLVMValue); islv && lv.pointer != nil {
		args[2] = c.builder.CreatePtrToInt(lv.pointer.LLVMValue(), ptrType, "")
	}
	if args[2].IsNil() {
		stackval := c.builder.CreateAlloca(c.types.ToLLVM(key.Type()), "")
		c.builder.CreateStore(key.LLVMValue(), stackval)
		args[2] = c.builder.CreatePtrToInt(stackval, ptrType, "")
	}
	c.builder.CreateCall(mapdelete, args, "")
}

// mapNext iterates through a map, accepting an iterator state value,
// and returning a new state value, key pointer, and value pointer.
func (c *compiler) mapNext(m *LLVMValue, nextin llvm.Value) (nextout, pk, pv llvm.Value) {
	mapnext := c.NamedFunction("runtime.mapnext", "func f(t, m, n uintptr) (uintptr, uintptr, uintptr)")
	mapType := underlyingType(m.Type()).(*types.Map)
	ptrType := c.target.IntPtrType()

	args := make([]llvm.Value, 3)
	args[0] = llvm.ConstPtrToInt(c.types.ToRuntime(mapType), ptrType)
	args[1] = c.builder.CreatePtrToInt(m.LLVMValue(), ptrType, "")
	args[2] = nextin
	results := c.builder.CreateCall(mapnext, args, "")
	nextout = c.builder.CreateExtractValue(results, 0, "")
	pk = c.builder.CreateExtractValue(results, 1, "")
	pv = c.builder.CreateExtractValue(results, 2, "")

	keyptrtype := &types.Pointer{Base: mapType.Key}
	valptrtype := &types.Pointer{Base: mapType.Elt}
	pk = c.builder.CreateIntToPtr(pk, c.types.ToLLVM(keyptrtype), "")
	pv = c.builder.CreateIntToPtr(pv, c.types.ToLLVM(valptrtype), "")

	return
}
