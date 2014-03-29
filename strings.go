// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/go-llvm/llvm"
	"go/token"
)

func (c *compiler) coerceString(v llvm.Value, typ llvm.Type) llvm.Value {
	result := llvm.Undef(typ)
	ptr := c.builder.CreateExtractValue(v, 0, "")
	len := c.builder.CreateExtractValue(v, 1, "")
	result = c.builder.CreateInsertValue(result, ptr, 0, "")
	result = c.builder.CreateInsertValue(result, len, 1, "")
	return result
}

func (fr *frame) concatenateStrings(lhs, rhs *LLVMValue) *LLVMValue {
	result := fr.runtime.stringPlus.call(fr, lhs.LLVMValue(), rhs.LLVMValue())
	return newValue(result[0], types.Typ[types.String])
}

func (fr *frame) compareStrings(lhs, rhs *LLVMValue, op token.Token) *LLVMValue {
	result := fr.runtime.strcmp.call(fr, lhs.LLVMValue(), rhs.LLVMValue())[0]
	zero := llvm.ConstNull(fr.types.inttype)
	var pred llvm.IntPredicate
	switch op {
	case token.EQL:
		pred = llvm.IntEQ
	case token.LSS:
		pred = llvm.IntSLT
	case token.GTR:
		pred = llvm.IntSGT
	case token.LEQ:
		pred = llvm.IntSLE
	case token.GEQ:
		pred = llvm.IntSGE
	case token.NEQ:
		panic("NEQ is handled in LLVMValue.BinaryOp")
	default:
		panic("unreachable")
	}
	result = fr.builder.CreateICmp(pred, result, zero, "")
	result = fr.builder.CreateZExt(result, llvm.Int8Type(), "")
	return newValue(result, types.Typ[types.Bool])
}

// stringIndex implements v = m[i]
func (c *compiler) stringIndex(s, i *LLVMValue) *LLVMValue {
	ptr := c.builder.CreateExtractValue(s.LLVMValue(), 0, "")
	ptr = c.builder.CreateGEP(ptr, []llvm.Value{i.LLVMValue()}, "")
	return newValue(c.builder.CreateLoad(ptr, ""), types.Typ[types.Byte])
}

func (fr *frame) stringIterInit(str *LLVMValue) []*LLVMValue {
	indexptr := fr.allocaBuilder.CreateAlloca(fr.types.inttype, "")
	fr.builder.CreateStore(llvm.ConstNull(fr.types.inttype), indexptr)
	return []*LLVMValue{str, newValue(indexptr, types.Typ[types.Int])}
}

// stringIterNext advances the iterator, and returns the tuple (ok, k, v).
func (fr *frame) stringIterNext(iter []*LLVMValue) []*LLVMValue {
	str, indexptr := iter[0], iter[1]
	k := fr.builder.CreateLoad(indexptr.LLVMValue(), "")

	result := fr.runtime.stringiter2.call(fr, str.LLVMValue(), k)
	fr.builder.CreateStore(result[0], indexptr.LLVMValue())
	ok := fr.builder.CreateIsNotNull(result[0], "")
	ok = fr.builder.CreateZExt(ok, llvm.Int8Type(), "")
	v := result[1]

	return []*LLVMValue{newValue(ok, types.Typ[types.Bool]), newValue(k, types.Typ[types.Int]), newValue(v, types.Typ[types.Rune])}
}

func (fr *frame) runeToString(v *LLVMValue) *LLVMValue {
	v = fr.convert(v, types.Typ[types.Int])
	result := fr.runtime.intToString.call(fr, v.LLVMValue())
	return newValue(result[0], types.Typ[types.String])
}

func (fr *frame) stringToRuneSlice(v *LLVMValue) *LLVMValue {
	result := fr.runtime.stringToIntArray.call(fr, v.LLVMValue())
	runeslice := types.NewSlice(types.Typ[types.Rune])
	return newValue(result[0], runeslice)
}

func (fr *frame) runeSliceToString(v *LLVMValue) *LLVMValue {
	llv := v.LLVMValue()
	ptr := fr.builder.CreateExtractValue(llv, 0, "")
	len := fr.builder.CreateExtractValue(llv, 1, "")
	result := fr.runtime.intArrayToString.call(fr, ptr, len)
	return newValue(result[0], types.Typ[types.String])
}
