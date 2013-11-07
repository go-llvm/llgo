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

	defer c.stackrestore(c.stacksave())
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
	if !commaOk {
		return c.NewValue(v, elemtyp)
	}
	fields := []*types.Var{
		types.NewParam(0, nil, "", elemtyp),
		types.NewParam(0, nil, "", types.Typ[types.Bool]),
	}
	typ := types.NewStruct(fields, nil)
	tuple := llvm.Undef(c.types.ToLLVM(typ))
	tuple = c.builder.CreateInsertValue(tuple, v, 0, "")
	tuple = c.builder.CreateInsertValue(tuple, ok, 1, "")
	return c.NewValue(tuple, typ)
}

// mapSlot gets a pointer to the m[k] value slot; if insert is true
// and the key does not exist, it is inserted.
func (c *compiler) mapSlot(m_, k_ *LLVMValue, insert_ bool) *LLVMValue {
	f := c.runtime.maplookup.LLVMValue()
	dyntyp := c.types.ToRuntime(m_.Type())
	dyntyp = c.builder.CreatePtrToInt(dyntyp, c.target.IntPtrType(), "")
	m := m_.LLVMValue()
	k := k_.LLVMValue()

	// FIXME require that k is a pointer
	defer c.stackrestore(c.stacksave())
	pk := c.builder.CreateAlloca(k.Type(), "")
	c.builder.CreateStore(k, pk)
	pk = c.builder.CreatePtrToInt(pk, c.target.IntPtrType(), "")

	insert := boolLLVMValue(insert_)
	ptrv := c.builder.CreateCall(f, []llvm.Value{dyntyp, m, pk, insert}, "")
	ptrvtyp := types.NewPointer(m_.Type().Underlying().(*types.Map).Elem())
	ptrv = c.builder.CreateIntToPtr(ptrv, c.types.ToLLVM(ptrvtyp), "")
	return c.NewValue(ptrv, ptrvtyp)
}

// mapUpdate implements m[k] = v
func (c *compiler) mapUpdate(m, k, v *LLVMValue) {
	ptrv := c.mapSlot(m, k, true)
	c.builder.CreateStore(v.LLVMValue(), ptrv.LLVMValue())
}
