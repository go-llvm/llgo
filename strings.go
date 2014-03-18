// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
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

func (c *compiler) concatenateStrings(lhs, rhs *LLVMValue) *LLVMValue {
	strcat := c.runtime.strcat.LLVMValue()
	_string := strcat.Type().ElementType().ReturnType()
	lhsstr := c.coerceString(lhs.LLVMValue(), _string)
	rhsstr := c.coerceString(rhs.LLVMValue(), _string)
	args := []llvm.Value{lhsstr, rhsstr}
	result := c.builder.CreateCall(strcat, args, "")
	result = c.coerceString(result, c.types.ToLLVM(types.Typ[types.String]))
	return c.NewValue(result, types.Typ[types.String])
}

func (c *compiler) compareStrings(lhs, rhs *LLVMValue, op token.Token) *LLVMValue {
	strcmp := c.runtime.strcmp.LLVMValue()
	_string := strcmp.Type().ElementType().ParamTypes()[0]
	lhsstr := c.coerceString(lhs.LLVMValue(), _string)
	rhsstr := c.coerceString(rhs.LLVMValue(), _string)
	args := []llvm.Value{lhsstr, rhsstr}
	result := c.builder.CreateCall(strcmp, args, "")
	zero := llvm.ConstNull(llvm.Int32Type())
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
	result = c.builder.CreateICmp(pred, result, zero, "")
	return c.NewValue(result, types.Typ[types.Bool])
}

// stringIndex implements v = m[i]
func (c *compiler) stringIndex(s, i *LLVMValue) *LLVMValue {
	ptr := c.builder.CreateExtractValue(s.LLVMValue(), 0, "")
	ptr = c.builder.CreateGEP(ptr, []llvm.Value{i.LLVMValue()}, "")
	return c.NewValue(c.builder.CreateLoad(ptr, ""), types.Typ[types.Byte])
}

// stringIterNext advances the iterator, and returns the tuple (ok, k, v).
func (c *compiler) stringIterNext(str *LLVMValue, preds []llvm.BasicBlock) *LLVMValue {
	// While Range/Next expresses a mutating operation, we represent them using
	// a Phi node where the first incoming branch (before the loop), and all
	// others take the previous value plus one.
	//
	// See ssa.go for comments on (and assertions of) our assumptions.
	index := c.builder.CreatePHI(c.types.inttype, "index")
	strnext := c.runtime.strnext.LLVMValue()
	args := []llvm.Value{
		c.coerceString(str.LLVMValue(), strnext.Type().ElementType().ParamTypes()[0]),
		index,
	}
	result := c.builder.CreateCall(strnext, args, "")
	nextindex := c.builder.CreateExtractValue(result, 0, "")
	runeval := c.builder.CreateExtractValue(result, 1, "")
	values := make([]llvm.Value, len(preds))
	values[0] = llvm.ConstNull(index.Type())
	for i, _ := range preds[1:] {
		values[i+1] = nextindex
	}
	index.AddIncoming(values, preds)

	// Create an (ok, index, rune) tuple.
	ok := c.builder.CreateIsNotNull(nextindex, "")
	typ := tupleType(types.Typ[types.Bool], types.Typ[types.Int], types.Typ[types.Rune])
	tuple := llvm.Undef(c.types.ToLLVM(typ))
	tuple = c.builder.CreateInsertValue(tuple, ok, 0, "")
	tuple = c.builder.CreateInsertValue(tuple, index, 1, "")
	tuple = c.builder.CreateInsertValue(tuple, runeval, 2, "")
	return c.NewValue(tuple, typ)
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
