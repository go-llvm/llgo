// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/go-llvm/llvm"
)

// makeChan implements make(chantype[, size])
func (fr *frame) makeChan(chantyp types.Type, size *LLVMValue) *LLVMValue {
	// TODO(pcc): call __go_new_channel_big here if needed
	dyntyp := fr.types.ToRuntime(chantyp)
	ch := fr.runtime.newChannel.call(fr, dyntyp, size.LLVMValue())[0]
	return fr.NewValue(ch, chantyp)
}

// chanSend implements ch<- x
func (fr *frame) chanSend(ch *LLVMValue, elem *LLVMValue) {
	elemtyp := ch.Type().Underlying().(*types.Chan).Elem()
	elem = fr.convert(elem, elemtyp)
	elemptr := fr.allocaBuilder.CreateAlloca(elem.LLVMValue().Type(), "")
	fr.builder.CreateStore(elem.LLVMValue(), elemptr)
	elemptr = fr.builder.CreateBitCast(elemptr, llvm.PointerType(llvm.Int8Type(), 0), "")
	chantyp := fr.types.ToRuntime(ch.Type())
	fr.runtime.sendBig.call(fr, chantyp, ch.LLVMValue(), elemptr)
}

// chanRecv implements x[, ok] = <-ch
func (fr *frame) chanRecv(ch *LLVMValue, commaOk bool) (x, ok *LLVMValue) {
	elemtyp := ch.Type().Underlying().(*types.Chan).Elem()
	ptr := fr.allocaBuilder.CreateAlloca(fr.types.ToLLVM(elemtyp), "")
	ptri8 := fr.builder.CreateBitCast(ptr, llvm.PointerType(llvm.Int8Type(), 0), "")
	chantyp := fr.types.ToRuntime(ch.Type())

	if commaOk {
		okval := fr.runtime.chanrecv2.call(fr, chantyp, ch.LLVMValue(), ptri8)[0]
		ok = fr.NewValue(okval, types.Typ[types.Bool])
	} else {
		fr.runtime.receiveBig.call(fr, chantyp, ch.LLVMValue(), ptri8)
	}
	x = fr.NewValue(fr.builder.CreateLoad(ptr, ""), elemtyp)
	return
}

// selectState is equivalent to ssa.SelectState
type selectState struct {
	Dir  types.ChanDir
	Chan *LLVMValue
	Send *LLVMValue
}

func (fr *frame) chanSelect(states []selectState, blocking bool) *LLVMValue {
	stackptr := fr.stacksave()
	defer fr.stackrestore(stackptr)

	n := uint64(len(states))
	if !blocking {
		// blocking means there's no default case
		n++
	}
	lln := llvm.ConstInt(llvm.Int32Type(), n, false)
	allocsize := fr.builder.CreateCall(fr.runtime.selectsize.LLVMValue(), []llvm.Value{lln}, "")
	selectp := fr.builder.CreateArrayAlloca(llvm.Int8Type(), allocsize, "selectp")
	fr.memsetZero(selectp, allocsize)
	selectp = fr.builder.CreatePtrToInt(selectp, fr.target.IntPtrType(), "")
	fr.builder.CreateCall(fr.runtime.selectinit.LLVMValue(), []llvm.Value{lln, selectp}, "")

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
	llResType := fr.types.ToLLVM(resType)
	tupleptr := fr.builder.CreateAlloca(llResType, "")
	fr.memsetZero(tupleptr, llvm.SizeOf(llResType))

	var recvindex int
	ptrs := make([]llvm.Value, len(states))
	for i, state := range states {
		chantyp := state.Chan.Type().Underlying().(*types.Chan)
		elemtyp := fr.types.ToLLVM(chantyp.Elem())
		if state.Dir == types.SendOnly {
			ptrs[i] = fr.builder.CreateAlloca(elemtyp, "")
			fr.builder.CreateStore(state.Send.LLVMValue(), ptrs[i])
		} else {
			ptrs[i] = fr.builder.CreateStructGEP(tupleptr, recvindex+2, "")
			recvindex++
		}
		ptrs[i] = fr.builder.CreatePtrToInt(ptrs[i], fr.target.IntPtrType(), "")
	}

	// Create select{send,recv} calls.
	selectsend := fr.runtime.selectsend.LLVMValue()
	selectrecv := fr.runtime.selectrecv.LLVMValue()
	var received llvm.Value
	if recvindex > 0 {
		received = fr.builder.CreateStructGEP(tupleptr, 1, "")
	}
	if !blocking {
		fr.builder.CreateCall(fr.runtime.selectdefault.LLVMValue(), []llvm.Value{selectp}, "")
	}
	for i, state := range states {
		ch := state.Chan.LLVMValue()
		if state.Dir == types.SendOnly {
			fr.builder.CreateCall(selectsend, []llvm.Value{selectp, ch, ptrs[i]}, "")
		} else {
			fr.builder.CreateCall(selectrecv, []llvm.Value{selectp, ch, ptrs[i], received}, "")
		}
	}

	// Fire off the select.
	index := fr.builder.CreateCall(fr.runtime.selectgo.LLVMValue(), []llvm.Value{selectp}, "")
	tuple := fr.builder.CreateLoad(tupleptr, "")
	tuple = fr.builder.CreateInsertValue(tuple, index, 0, "")
	return fr.NewValue(tuple, resType)
}
