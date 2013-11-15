// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// makeMap implements make(maptype[, initial space])
func (c *compiler) makeMap(typ types.Type, cap_ *LLVMValue) *LLVMValue {
	f := c.runtime.makemap.LLVMValue()
	dyntyp := c.types.ToRuntime(typ)
	dyntyp = c.builder.CreatePtrToInt(dyntyp, c.target.IntPtrType(), "")
	var cap llvm.Value
	if cap_ != nil {
		cap = cap_.Convert(types.Typ[types.Int]).LLVMValue()
	} else {
		cap = llvm.ConstNull(c.types.inttype)
	}
	m := c.builder.CreateCall(f, []llvm.Value{dyntyp, cap}, "")
	return c.NewValue(m, typ)
}

// mapLookup implements v[, ok] = m[k]
func (c *compiler) mapLookup(m, k *LLVMValue, commaOk bool) *LLVMValue {
	dyntyp := c.types.ToRuntime(m.Type())
	dyntyp = c.builder.CreatePtrToInt(dyntyp, c.target.IntPtrType(), "")

	stackptr := c.stacksave()
	llk := k.LLVMValue()
	pk := c.builder.CreateAlloca(llk.Type(), "")
	c.builder.CreateStore(llk, pk)
	elemtyp := m.Type().Underlying().(*types.Map).Elem()
	pv := c.builder.CreateAlloca(c.types.ToLLVM(elemtyp), "")
	ok := c.builder.CreateCall(
		c.runtime.mapaccess.LLVMValue(),
		[]llvm.Value{
			dyntyp,
			m.LLVMValue(),
			c.builder.CreatePtrToInt(pk, c.target.IntPtrType(), ""),
			c.builder.CreatePtrToInt(pv, c.target.IntPtrType(), ""),
		}, "",
	)
	v := c.builder.CreateLoad(pv, "")
	c.stackrestore(stackptr)
	if !commaOk {
		return c.NewValue(v, elemtyp)
	}

	typ := tupleType(elemtyp, types.Typ[types.Bool])
	tuple := llvm.Undef(c.types.ToLLVM(typ))
	tuple = c.builder.CreateInsertValue(tuple, v, 0, "")
	tuple = c.builder.CreateInsertValue(tuple, ok, 1, "")
	return c.NewValue(tuple, typ)
}

// mapUpdate implements m[k] = v
func (c *compiler) mapUpdate(m_, k_, v_ *LLVMValue) {
	f := c.runtime.maplookup.LLVMValue()
	dyntyp := c.types.ToRuntime(m_.Type())
	dyntyp = c.builder.CreatePtrToInt(dyntyp, c.target.IntPtrType(), "")
	m := m_.LLVMValue()
	k := k_.LLVMValue()

	stackptr := c.stacksave()
	pk := c.builder.CreateAlloca(k.Type(), "")
	c.builder.CreateStore(k, pk)
	pk = c.builder.CreatePtrToInt(pk, c.target.IntPtrType(), "")
	insert := boolLLVMValue(true)
	ptrv := c.builder.CreateCall(f, []llvm.Value{dyntyp, m, pk, insert}, "")
	c.stackrestore(stackptr)

	ptrvtyp := types.NewPointer(m_.Type().Underlying().(*types.Map).Elem())
	ptrv = c.builder.CreateIntToPtr(ptrv, c.types.ToLLVM(ptrvtyp), "")
	c.builder.CreateStore(v_.LLVMValue(), ptrv)
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
	ok := c.builder.CreateCall(mapiternext, []llvm.Value{lliter}, "")

	typ := tupleType(types.Typ[types.Bool], ktyp, vtyp)
	tuple := llvm.Undef(c.types.ToLLVM(typ))
	tuple = c.builder.CreateInsertValue(tuple, ok, 0, "")

	currBlock := c.builder.GetInsertBlock()
	endBlock := llvm.InsertBasicBlock(currBlock, "")
	endBlock.MoveAfter(currBlock)
	okBlock := llvm.InsertBasicBlock(endBlock, "")
	c.builder.CreateCondBr(ok, okBlock, endBlock)

	// TODO when mapIterInit/mapIterNext operate on a
	// stack-allocated struct, use CreateExtractValue
	// to load pk/pv.
	c.builder.SetInsertPointAtEnd(okBlock)
	lliter = c.builder.CreateIntToPtr(lliter, llvm.PointerType(c.runtime.mapiter.llvm, 0), "")
	pk := c.builder.CreateLoad(c.builder.CreateStructGEP(lliter, 0, ""), "")
	pv := c.builder.CreateLoad(c.builder.CreateStructGEP(lliter, 1, ""), "")
	k := c.builder.CreateLoad(c.builder.CreateIntToPtr(pk, llvm.PointerType(c.types.ToLLVM(ktyp), 0), ""), "")
	v := c.builder.CreateLoad(c.builder.CreateIntToPtr(pv, llvm.PointerType(c.types.ToLLVM(vtyp), 0), ""), "")
	tuplekv := c.builder.CreateInsertValue(tuple, k, 1, "")
	tuplekv = c.builder.CreateInsertValue(tuplekv, v, 2, "")
	c.builder.CreateBr(endBlock)

	c.builder.SetInsertPointAtEnd(endBlock)
	phi := c.builder.CreatePHI(tuple.Type(), "")
	phi.AddIncoming([]llvm.Value{tuple, tuplekv}, []llvm.BasicBlock{currBlock, okBlock})
	return c.NewValue(phi, typ)
}
