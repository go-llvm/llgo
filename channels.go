// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// makeChan implements make(chantype[, size])
func (c *compiler) makeChan(chantyp types.Type, size *LLVMValue) *LLVMValue {
	f := c.runtime.makechan.LLVMValue()
	dyntyp := c.types.ToRuntime(chantyp)
	dyntyp = c.builder.CreatePtrToInt(dyntyp, c.target.IntPtrType(), "")
	ch := c.builder.CreateCall(f, []llvm.Value{dyntyp, size.LLVMValue()}, "")
	return c.NewValue(ch, chantyp)
}

// chanSend implements ch<- x
func (c *compiler) chanSend(ch *LLVMValue, elem *LLVMValue) {
	elemtyp := ch.Type().Underlying().(*types.Chan).Elem()
	elem = elem.Convert(elemtyp).(*LLVMValue)
	stackptr := c.stacksave()
	elemptr := c.builder.CreateAlloca(elem.LLVMValue().Type(), "")
	c.builder.CreateStore(elem.LLVMValue(), elemptr)
	elemptr = c.builder.CreatePtrToInt(elemptr, c.target.IntPtrType(), "")
	nb := boolLLVMValue(false)
	chansend := c.runtime.chansend.LLVMValue()
	chantyp := c.types.ToRuntime(ch.typ.Underlying())
	chantyp = c.builder.CreateBitCast(chantyp, chansend.Type().ElementType().ParamTypes()[0], "")
	c.builder.CreateCall(chansend, []llvm.Value{chantyp, ch.LLVMValue(), elemptr, nb}, "")
	// Ignore result; only used in runtime.
	c.stackrestore(stackptr)
}

// chanRecv implements x[, ok] = <-ch
func (c *compiler) chanRecv(ch *LLVMValue, commaOk bool) *LLVMValue {
	elemtyp := ch.Type().Underlying().(*types.Chan).Elem()
	stackptr := c.stacksave()
	ptr := c.builder.CreateAlloca(c.types.ToLLVM(elemtyp), "")
	chanrecv := c.runtime.chanrecv.LLVMValue()
	chantyp := c.types.ToRuntime(ch.Type().Underlying())
	chantyp = c.builder.CreateBitCast(chantyp, chanrecv.Type().ElementType().ParamTypes()[0], "")
	ok := c.builder.CreateCall(chanrecv, []llvm.Value{
		chantyp,
		ch.LLVMValue(),
		c.builder.CreatePtrToInt(ptr, c.target.IntPtrType(), ""),
		boolLLVMValue(false), // nb
	}, "")
	elem := c.builder.CreateLoad(ptr, "")
	c.stackrestore(stackptr)
	if !commaOk {
		return c.NewValue(elem, elemtyp)
	}
	typ := tupleType(elemtyp, types.Typ[types.Bool])
	tuple := llvm.Undef(c.types.ToLLVM(typ))
	tuple = c.builder.CreateInsertValue(tuple, elem, 0, "")
	tuple = c.builder.CreateInsertValue(tuple, ok, 1, "")
	return c.NewValue(tuple, typ)
}
