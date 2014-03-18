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
func (fr *frame) chanSend(ch *LLVMValue, elem *LLVMValue) {
	elemtyp := ch.Type().Underlying().(*types.Chan).Elem()
	elem = fr.convert(elem, elemtyp).(*LLVMValue)
	stackptr := fr.stacksave()
	elemptr := fr.builder.CreateAlloca(elem.LLVMValue().Type(), "")
	fr.builder.CreateStore(elem.LLVMValue(), elemptr)
	elemptr = fr.builder.CreatePtrToInt(elemptr, fr.target.IntPtrType(), "")
	nb := boolLLVMValue(false)
	chansend := fr.runtime.chansend.LLVMValue()
	chantyp := fr.types.ToRuntime(ch.typ.Underlying())
	chantyp = fr.builder.CreateBitCast(chantyp, chansend.Type().ElementType().ParamTypes()[0], "")
	fr.builder.CreateCall(chansend, []llvm.Value{chantyp, ch.LLVMValue(), elemptr, nb}, "")
	// Ignore result; only used in runtime.
	fr.stackrestore(stackptr)
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

// selectState is equivalent to ssa.SelectState
type selectState struct {
	Dir  types.ChanDir
	Chan *LLVMValue
	Send *LLVMValue
}

func (c *compiler) chanSelect(states []selectState, blocking bool) *LLVMValue {
	stackptr := c.stacksave()
	defer c.stackrestore(stackptr)

	n := uint64(len(states))
	if !blocking {
		// blocking means there's no default case
		n++
	}
	lln := llvm.ConstInt(llvm.Int32Type(), n, false)
	allocsize := c.builder.CreateCall(c.runtime.selectsize.LLVMValue(), []llvm.Value{lln}, "")
	selectp := c.builder.CreateArrayAlloca(llvm.Int8Type(), allocsize, "selectp")
	c.memsetZero(selectp, allocsize)
	selectp = c.builder.CreatePtrToInt(selectp, c.target.IntPtrType(), "")
	c.builder.CreateCall(c.runtime.selectinit.LLVMValue(), []llvm.Value{lln, selectp}, "")

	// Allocate stack for the values to send/receive.
	//
	// TODO(axw) request optimisation in ssa to special-
	// case receive cases with no assignment, so we know
	// not to allocate stack space or do a copy.
	resTypes := []types.Type{types.Typ[types.Int], types.Typ[types.Bool]}
	for _, state := range states {
		if state.Dir == types.RecvOnly {
			chantyp := state.Chan.Type().Underlying().(*types.Chan)
			resTypes = append(resTypes, chantyp.Elem())
		}
	}
	resType := tupleType(resTypes...)
	llResType := c.types.ToLLVM(resType)
	tupleptr := c.builder.CreateAlloca(llResType, "")
	c.memsetZero(tupleptr, llvm.SizeOf(llResType))

	var recvindex int
	ptrs := make([]llvm.Value, len(states))
	for i, state := range states {
		chantyp := state.Chan.Type().Underlying().(*types.Chan)
		elemtyp := c.types.ToLLVM(chantyp.Elem())
		if state.Dir == types.SendOnly {
			ptrs[i] = c.builder.CreateAlloca(elemtyp, "")
			c.builder.CreateStore(state.Send.LLVMValue(), ptrs[i])
		} else {
			ptrs[i] = c.builder.CreateStructGEP(tupleptr, recvindex+2, "")
			recvindex++
		}
		ptrs[i] = c.builder.CreatePtrToInt(ptrs[i], c.target.IntPtrType(), "")
	}

	// Create select{send,recv} calls.
	selectsend := c.runtime.selectsend.LLVMValue()
	selectrecv := c.runtime.selectrecv.LLVMValue()
	var received llvm.Value
	if recvindex > 0 {
		received = c.builder.CreateStructGEP(tupleptr, 1, "")
	}
	if !blocking {
		c.builder.CreateCall(c.runtime.selectdefault.LLVMValue(), []llvm.Value{selectp}, "")
	}
	for i, state := range states {
		ch := state.Chan.LLVMValue()
		if state.Dir == types.SendOnly {
			c.builder.CreateCall(selectsend, []llvm.Value{selectp, ch, ptrs[i]}, "")
		} else {
			c.builder.CreateCall(selectrecv, []llvm.Value{selectp, ch, ptrs[i], received}, "")
		}
	}

	// Fire off the select.
	index := c.builder.CreateCall(c.runtime.selectgo.LLVMValue(), []llvm.Value{selectp}, "")
	tuple := c.builder.CreateLoad(tupleptr, "")
	tuple = c.builder.CreateInsertValue(tuple, index, 0, "")
	return c.NewValue(tuple, resType)
}
