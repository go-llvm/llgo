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
	f := fr.runtime.makemap.LLVMValue()
	dyntyp := fr.types.ToRuntime(typ)
	dyntyp = fr.builder.CreatePtrToInt(dyntyp, fr.target.IntPtrType(), "")
	var cap llvm.Value
	if cap_ != nil {
		cap = fr.convert(cap_, types.Typ[types.Int]).LLVMValue()
	} else {
		cap = llvm.ConstNull(fr.types.inttype)
	}
	m := fr.builder.CreateCall(f, []llvm.Value{dyntyp, cap}, "")
	return fr.NewValue(m, typ)
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
