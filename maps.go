// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// makeMap implements make(maptype[, initial space])
func (fr *frame) makeMap(typ types.Type, cap_ *LLVMValue) *LLVMValue {
	dyntyp := fr.types.getMapDescriptorPointer(typ)
	dyntyp = fr.builder.CreateBitCast(dyntyp, llvm.PointerType(llvm.Int8Type(), 0), "")
	var cap llvm.Value
	if cap_ != nil {
		cap = fr.convert(cap_, types.Typ[types.Uintptr]).LLVMValue()
	} else {
		cap = llvm.ConstNull(fr.types.inttype)
	}
	m := fr.runtime.newMap.call(fr, dyntyp, cap)
	return fr.NewValue(m[0], typ)
}

// mapLookup implements v[, ok] = m[k]
func (fr *frame) mapLookup(m, k *LLVMValue) (v *LLVMValue, ok *LLVMValue) {
	llk := k.LLVMValue()
	pk := fr.allocaBuilder.CreateAlloca(llk.Type(), "")
	fr.builder.CreateStore(llk, pk)
	valptr := fr.runtime.mapIndex.call(fr, m.LLVMValue(), pk, boolLLVMValue(false))[0]

	okbit := fr.builder.CreateIsNotNull(valptr, "")
	ok = fr.NewValue(fr.builder.CreateZExt(okbit, llvm.Int8Type(), ""), types.Typ[types.Bool])
	startbb := fr.builder.GetInsertBlock()
	loadbb := llvm.AddBasicBlock(fr.function, "")
	contbb := llvm.AddBasicBlock(fr.function, "")
	fr.builder.CreateCondBr(okbit, loadbb, contbb)

	fr.builder.SetInsertPointAtEnd(loadbb)
	elemtyp := m.Type().Underlying().(*types.Map).Elem()
	llelemtyp := fr.types.ToLLVM(elemtyp)
	typedvalptr := fr.builder.CreateBitCast(valptr, llvm.PointerType(llelemtyp, 0), "")
	loadedval := fr.builder.CreateLoad(typedvalptr, "")
	fr.builder.CreateBr(contbb)

	fr.builder.SetInsertPointAtEnd(contbb)
	llv := fr.builder.CreatePHI(llelemtyp, "")
	llv.AddIncoming([]llvm.Value{llvm.ConstNull(llelemtyp), loadedval},
	                []llvm.BasicBlock{startbb, loadbb})
	v = fr.NewValue(llv, elemtyp)
	return
}

// mapUpdate implements m[k] = v
func (fr *frame) mapUpdate(m, k, v *LLVMValue) {
	llk := k.LLVMValue()
	pk := fr.allocaBuilder.CreateAlloca(llk.Type(), "")
	fr.builder.CreateStore(llk, pk)
	valptr := fr.runtime.mapIndex.call(fr, m.LLVMValue(), pk, boolLLVMValue(true))[0]

	elemtyp := m.Type().Underlying().(*types.Map).Elem()
	llelemtyp := fr.types.ToLLVM(elemtyp)
	typedvalptr := fr.builder.CreateBitCast(valptr, llvm.PointerType(llelemtyp, 0), "")
	fr.builder.CreateStore(v.LLVMValue(), typedvalptr)
}

// mapDelete implements delete(m, k)
func (fr *frame) mapDelete(m, k *LLVMValue) {
	llk := k.LLVMValue()
	pk := fr.allocaBuilder.CreateAlloca(llk.Type(), "")
	fr.builder.CreateStore(llk, pk)
	fr.runtime.mapdelete.call(fr, m.LLVMValue(), pk)
}

// mapIterInit creates a map iterator
func (c *compiler) mapIterInit(m *LLVMValue) *LLVMValue {
	// TODO allocate iterator on stack at usage site
	f := c.runtime.mapiterinit.LLVMValue()
	dyntyp := c.types.ToRuntime(m.Type())
	iter := c.builder.CreateCall(f, []llvm.Value{
		c.builder.CreatePtrToInt(dyntyp, c.target.IntPtrType(), ""),
		c.builder.CreatePtrToInt(m.LLVMValue(), c.target.IntPtrType(), ""),
	}, "")
	return c.NewValue(iter, m.Type())
}

// mapIterNext advances the iterator, and returns the tuple (ok, k, v).
func (c *compiler) mapIterNext(iter *LLVMValue) *LLVMValue {
	maptyp := iter.Type().Underlying().(*types.Map)
	ktyp := maptyp.Key()
	vtyp := maptyp.Elem()
	lliter := iter.LLVMValue()
	mapiternext := c.runtime.mapiternext.LLVMValue()
	typ := tupleType(types.Typ[types.Bool], ktyp, vtyp)
	// allocate space for the key and value on the stack, and pass
	// pointers into maliternext.
	stackptr := c.stacksave()
	tuple := c.builder.CreateAlloca(c.types.ToLLVM(typ), "")
	pk := c.builder.CreatePtrToInt(c.builder.CreateStructGEP(tuple, 1, ""), c.target.IntPtrType(), "")
	pv := c.builder.CreatePtrToInt(c.builder.CreateStructGEP(tuple, 2, ""), c.target.IntPtrType(), "")
	ok := c.builder.CreateCall(mapiternext, []llvm.Value{lliter, pk, pv}, "")
	c.builder.CreateStore(ok, c.builder.CreateStructGEP(tuple, 0, ""))
	tuple = c.builder.CreateLoad(tuple, "")
	c.stackrestore(stackptr)
	return c.NewValue(tuple, typ)
}
