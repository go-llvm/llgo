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
	return fr.NewValue(result[0], types.Typ[types.String])
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
	return fr.NewValue(result, types.Typ[types.Bool])
}

// stringIndex implements v = m[i]
func (c *compiler) stringIndex(s, i *LLVMValue) *LLVMValue {
	ptr := c.builder.CreateExtractValue(s.LLVMValue(), 0, "")
	ptr = c.builder.CreateGEP(ptr, []llvm.Value{i.LLVMValue()}, "")
	return c.NewValue(c.builder.CreateLoad(ptr, ""), types.Typ[types.Byte])
}

func (fr *frame) stringIterInit(str *LLVMValue) []*LLVMValue {
	indexptr := fr.allocaBuilder.CreateAlloca(fr.types.inttype, "")
	fr.builder.CreateStore(llvm.ConstNull(fr.types.inttype), indexptr)
	return []*LLVMValue{str, fr.NewValue(indexptr, types.Typ[types.Int])}
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

	return []*LLVMValue{fr.NewValue(ok, types.Typ[types.Bool]), fr.NewValue(k, types.Typ[types.Int]), fr.NewValue(v, types.Typ[types.Rune])}
}

func (fr *frame) runeToString(v *LLVMValue) *LLVMValue {
	c := v.compiler
	strrune := c.runtime.strrune.LLVMValue()
	args := []llvm.Value{fr.convert(v, types.Typ[types.Int64]).LLVMValue()}
	result := c.builder.CreateCall(strrune, args, "")
	result = c.coerceString(result, c.types.ToLLVM(types.Typ[types.String]))
	return c.NewValue(result, types.Typ[types.String])
}

func (v *LLVMValue) stringToRuneSlice() *LLVMValue {
	c := v.compiler
	strtorunes := c.runtime.strtorunes.LLVMValue()
	_string := strtorunes.Type().ElementType().ParamTypes()[0]
	args := []llvm.Value{c.coerceString(v.LLVMValue(), _string)}
	result := c.builder.CreateCall(strtorunes, args, "")
	runeslice := types.NewSlice(types.Typ[types.Rune])
	result = c.coerceSlice(result, c.types.ToLLVM(runeslice))
	return c.NewValue(result, runeslice)
}

func (v *LLVMValue) runeSliceToString() *LLVMValue {
	c := v.compiler
	runestostr := c.runtime.runestostr.LLVMValue()
	i8slice := runestostr.Type().ElementType().ParamTypes()[0]
	args := []llvm.Value{c.coerceSlice(v.LLVMValue(), i8slice)}
	result := c.builder.CreateCall(runestostr, args, "")
	result = c.coerceString(result, c.types.ToLLVM(types.Typ[types.String]))
	return c.NewValue(result, types.Typ[types.String])
}
