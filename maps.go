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

	elemtyp := m.Type().Underlying().(*types.Map).Elem()
	ok = fr.NewValue(fr.builder.CreateZExt(okbit, llvm.Int8Type(), ""), types.Typ[types.Bool])
	v = fr.loadOrNull(okbit, valptr, elemtyp)
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
func (fr *frame) mapIterInit(m *LLVMValue) []*LLVMValue {
	// We represent an iterator as a tuple (map, *bool). The second element
	// controls whether the code we generate for "next" (below) calls the
	// runtime function for the first or the next element. We let the
	// optimizer reorganize this into something more sensible.
	isinit := fr.allocaBuilder.CreateAlloca(llvm.Int1Type(), "")
	fr.builder.CreateStore(llvm.ConstNull(llvm.Int1Type()), isinit)

	return []*LLVMValue{m, fr.NewValue(isinit, types.NewPointer(types.Typ[types.Bool]))}
}

// mapIterNext advances the iterator, and returns the tuple (ok, k, v).
func (fr *frame) mapIterNext(iter []*LLVMValue) []*LLVMValue {
	maptyp := iter[0].Type().Underlying().(*types.Map)
	ktyp := maptyp.Key()
	klltyp := fr.types.ToLLVM(ktyp)
	vtyp := maptyp.Elem()
	vlltyp := fr.types.ToLLVM(vtyp)

	m, isinitptr := iter[0], iter[1]

	i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
	mapiterbufty := llvm.ArrayType(i8ptr, 4)
	mapiterbuf := fr.allocaBuilder.CreateAlloca(mapiterbufty, "")
	mapiterbufelem0ptr := fr.builder.CreateStructGEP(mapiterbuf, 0, "")

	keybuf := fr.allocaBuilder.CreateAlloca(klltyp, "")
	keyptr := fr.builder.CreateBitCast(keybuf, i8ptr, "")
	valbuf := fr.allocaBuilder.CreateAlloca(vlltyp, "")
	valptr := fr.builder.CreateBitCast(valbuf, i8ptr, "")

	isinit := fr.builder.CreateLoad(isinitptr.LLVMValue(), "")

	initbb := llvm.AddBasicBlock(fr.function, "")
	nextbb := llvm.AddBasicBlock(fr.function, "")
	contbb := llvm.AddBasicBlock(fr.function, "")

	fr.builder.CreateCondBr(isinit, nextbb, initbb)

	fr.builder.SetInsertPointAtEnd(initbb)
	fr.builder.CreateStore(llvm.ConstAllOnes(llvm.Int1Type()), isinitptr.LLVMValue())
	fr.runtime.mapiterinit.call(fr, m.LLVMValue(), mapiterbufelem0ptr)
	fr.builder.CreateBr(contbb)

	fr.builder.SetInsertPointAtEnd(nextbb)
	fr.runtime.mapiternext.call(fr, mapiterbufelem0ptr)
	fr.builder.CreateBr(contbb)

	fr.builder.SetInsertPointAtEnd(contbb)
	mapiterbufelem0 := fr.builder.CreateLoad(mapiterbufelem0ptr, "")
	okbit := fr.builder.CreateIsNotNull(mapiterbufelem0, "")
	ok := fr.builder.CreateZExt(okbit, llvm.Int8Type(), "")

	loadbb := llvm.AddBasicBlock(fr.function, "")
	cont2bb := llvm.AddBasicBlock(fr.function, "")
	fr.builder.CreateCondBr(okbit, loadbb, cont2bb)

	fr.builder.SetInsertPointAtEnd(loadbb)
	fr.runtime.mapiter2.call(fr, mapiterbufelem0ptr, keyptr, valptr)
	loadedkey := fr.builder.CreateLoad(keybuf, "")
	loadedval := fr.builder.CreateLoad(valbuf, "")
	fr.builder.CreateBr(cont2bb)

	fr.builder.SetInsertPointAtEnd(cont2bb)
	k := fr.builder.CreatePHI(klltyp, "")
	k.AddIncoming([]llvm.Value{llvm.ConstNull(klltyp), loadedkey},
	              []llvm.BasicBlock{contbb, loadbb})
	v := fr.builder.CreatePHI(vlltyp, "")
	v.AddIncoming([]llvm.Value{llvm.ConstNull(vlltyp), loadedval},
	              []llvm.BasicBlock{contbb, loadbb})

	return []*LLVMValue{fr.NewValue(ok, types.Typ[types.Bool]), fr.NewValue(k, ktyp), fr.NewValue(v, vtyp)}
}
