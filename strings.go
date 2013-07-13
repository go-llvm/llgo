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
	strcat := c.NamedFunction("runtime.strcat", "func(a, b _string) _string")
	_string := strcat.Type().ElementType().ReturnType()
	lhsstr := c.coerceString(lhs.LLVMValue(), _string)
	rhsstr := c.coerceString(rhs.LLVMValue(), _string)
	args := []llvm.Value{lhsstr, rhsstr}
	result := c.builder.CreateCall(strcat, args, "")
	result = c.coerceString(result, c.types.ToLLVM(types.Typ[types.String]))
	return c.NewValue(result, types.Typ[types.String])
}

func (c *compiler) compareStrings(lhs, rhs *LLVMValue, op token.Token) *LLVMValue {
	strcmp := c.NamedFunction("runtime.strcmp", "func(a, b _string) int32")
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

func (c *compiler) stringNext(strval, index llvm.Value) (consumed, value llvm.Value) {
	strnext := c.NamedFunction("runtime.strnext", "func(s _string, i int) (int, rune)")
	_string := strnext.Type().ElementType().ParamTypes()[0]
	strval = c.coerceString(strval, _string)
	args := []llvm.Value{strval, index}
	result := c.builder.CreateCall(strnext, args, "")
	consumed = c.builder.CreateExtractValue(result, 0, "")
	value = c.builder.CreateExtractValue(result, 1, "")
	return
}

func (v *LLVMValue) runeToString() *LLVMValue {
	c := v.compiler
	strrune := c.NamedFunction("runtime.strrune", "func(n int64) _string")
	args := []llvm.Value{v.Convert(types.Typ[types.Int64]).LLVMValue()}
	result := c.builder.CreateCall(strrune, args, "")
	result = c.coerceString(result, c.types.ToLLVM(types.Typ[types.String]))
	return c.NewValue(result, types.Typ[types.String])
}

func (v *LLVMValue) stringToRuneSlice() *LLVMValue {
	c := v.compiler
	strtorunes := c.NamedFunction("runtime.strtorunes", "func(_string) slice")
	_string := strtorunes.Type().ElementType().ParamTypes()[0]
	args := []llvm.Value{c.coerceString(v.LLVMValue(), _string)}
	result := c.builder.CreateCall(strtorunes, args, "")
	runeslice := types.NewSlice(types.Typ[types.Rune])
	result = c.coerceSlice(result, c.types.ToLLVM(runeslice))
	return c.NewValue(result, runeslice)
}

func (v *LLVMValue) runeSliceToString() *LLVMValue {
	c := v.compiler
	runestostr := c.NamedFunction("runtime.runestostr", "func(slice) _string")
	i8slice := runestostr.Type().ElementType().ParamTypes()[0]
	args := []llvm.Value{c.coerceSlice(v.LLVMValue(), i8slice)}
	result := c.builder.CreateCall(runestostr, args, "")
	result = c.coerceString(result, c.types.ToLLVM(types.Typ[types.String]))
	return c.NewValue(result, types.Typ[types.String])
}

// vim: set ft=go:
