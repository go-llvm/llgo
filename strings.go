// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/go-llvm/llvm"
	"go/token"
)

func (fr *frame) concatenateStrings(lhs, rhs *govalue) *govalue {
	result := fr.runtime.stringPlus.call(fr, lhs.value, rhs.value)
	return newValue(result[0], types.Typ[types.String])
}

func (fr *frame) compareStrings(lhs, rhs *govalue, op token.Token) *govalue {
	result := fr.runtime.strcmp.call(fr, lhs.value, rhs.value)[0]
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
		panic("NEQ is handled in govalue.BinaryOp")
	default:
		panic("unreachable")
	}
	result = fr.builder.CreateICmp(pred, result, zero, "")
	result = fr.builder.CreateZExt(result, llvm.Int8Type(), "")
	return newValue(result, types.Typ[types.Bool])
}

// stringIndex implements v = m[i]
func (c *compiler) stringIndex(s, i *govalue) *govalue {
	ptr := c.builder.CreateExtractValue(s.value, 0, "")
	ptr = c.builder.CreateGEP(ptr, []llvm.Value{i.value}, "")
	return newValue(c.builder.CreateLoad(ptr, ""), types.Typ[types.Byte])
}

func (fr *frame) stringIterInit(str *govalue) []*govalue {
	indexptr := fr.allocaBuilder.CreateAlloca(fr.types.inttype, "")
	fr.builder.CreateStore(llvm.ConstNull(fr.types.inttype), indexptr)
	return []*govalue{str, newValue(indexptr, types.Typ[types.Int])}
}

// stringIterNext advances the iterator, and returns the tuple (ok, k, v).
func (fr *frame) stringIterNext(iter []*govalue) []*govalue {
	str, indexptr := iter[0], iter[1]
	k := fr.builder.CreateLoad(indexptr.value, "")

	result := fr.runtime.stringiter2.call(fr, str.value, k)
	fr.builder.CreateStore(result[0], indexptr.value)
	ok := fr.builder.CreateIsNotNull(result[0], "")
	ok = fr.builder.CreateZExt(ok, llvm.Int8Type(), "")
	v := result[1]

	return []*govalue{newValue(ok, types.Typ[types.Bool]), newValue(k, types.Typ[types.Int]), newValue(v, types.Typ[types.Rune])}
}

func (fr *frame) runeToString(v *govalue) *govalue {
	v = fr.convert(v, types.Typ[types.Int])
	result := fr.runtime.intToString.call(fr, v.value)
	return newValue(result[0], types.Typ[types.String])
}

func (fr *frame) stringToRuneSlice(v *govalue) *govalue {
	result := fr.runtime.stringToIntArray.call(fr, v.value)
	runeslice := types.NewSlice(types.Typ[types.Rune])
	return newValue(result[0], runeslice)
}

func (fr *frame) runeSliceToString(v *govalue) *govalue {
	llv := v.value
	ptr := fr.builder.CreateExtractValue(llv, 0, "")
	len := fr.builder.CreateExtractValue(llv, 1, "")
	result := fr.runtime.intArrayToString.call(fr, ptr, len)
	return newValue(result[0], types.Typ[types.String])
}
