// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"fmt"

	"code.google.com/p/go.tools/go/exact"
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// convertE2V converts (type asserts) an interface value to a concrete type.
func (v *LLVMValue) convertE2V(typ types.Type) (result, success *LLVMValue) {
	builder := v.compiler.builder
	predicate := v.interfaceTypeEquals(typ).LLVMValue()

	// If result is zero, then we've got a match.
	end := llvm.InsertBasicBlock(builder.GetInsertBlock(), "end")
	end.MoveAfter(builder.GetInsertBlock())
	nonmatch := llvm.InsertBasicBlock(end, "nonmatch")
	match := llvm.InsertBasicBlock(nonmatch, "match")
	builder.CreateCondBr(predicate, match, nonmatch)

	builder.SetInsertPointAtEnd(match)
	matchResultValue := v.loadE2V(typ).LLVMValue()
	builder.CreateBr(end)

	builder.SetInsertPointAtEnd(nonmatch)
	nonmatchResultValue := llvm.ConstNull(matchResultValue.Type())
	builder.CreateBr(end)

	builder.SetInsertPointAtEnd(end)
	successValue := builder.CreatePHI(llvm.Int1Type(), "")
	resultValue := builder.CreatePHI(matchResultValue.Type(), "")

	successValues := []llvm.Value{llvm.ConstAllOnes(llvm.Int1Type()), llvm.ConstNull(llvm.Int1Type())}
	successBlocks := []llvm.BasicBlock{match, nonmatch}
	successValue.AddIncoming(successValues, successBlocks)
	success = v.compiler.NewValue(successValue, types.Typ[types.Bool])

	resultValues := []llvm.Value{matchResultValue, nonmatchResultValue}
	resultBlocks := []llvm.BasicBlock{match, nonmatch}
	resultValue.AddIncoming(resultValues, resultBlocks)
	result = v.compiler.NewValue(resultValue, typ)
	return result, success
}

// convertI2E converts a non-empty interface value to an empty interface.
func (v *LLVMValue) convertI2E() *LLVMValue {
	c := v.compiler
	f := c.runtime.convertI2E.LLVMValue()
	args := []llvm.Value{c.coerce(v.LLVMValue(), c.runtime.iface.llvm)}
	return c.NewValue(c.builder.CreateCall(f, args, ""), types.NewInterface(nil, nil))
}

// convertE2I converts an empty interface value to a non-empty interface.
func (v *LLVMValue) convertE2I(iface types.Type) (result, success *LLVMValue) {
	c := v.compiler
	f := c.runtime.convertE2I.LLVMValue()
	typ := c.builder.CreatePtrToInt(c.types.ToRuntime(iface), c.target.IntPtrType(), "")
	args := []llvm.Value{c.coerce(v.LLVMValue(), c.runtime.eface.llvm), typ}
	res := c.coerce(c.builder.CreateCall(f, args, ""), c.types.ToLLVM(iface))
	succ := c.builder.CreateIsNotNull(c.builder.CreateExtractValue(res, 0, ""), "")
	return c.NewValue(res, iface), c.NewValue(succ, types.Typ[types.Bool])
}

// mustConvertE2I calls convertE2I, panicking if the assertion failed.
func (v *LLVMValue) mustConvertE2I(typ types.Type) *LLVMValue {
	result, ok := v.convertE2I(typ)
	builder := v.compiler.builder
	end := llvm.InsertBasicBlock(builder.GetInsertBlock(), "end")
	end.MoveAfter(builder.GetInsertBlock())
	failed := llvm.InsertBasicBlock(end, "failed")
	builder.CreateCondBr(ok.LLVMValue(), end, failed)
	builder.SetInsertPointAtEnd(failed)
	c := v.compiler
	s := fmt.Sprintf("convertE2I(%s, %s) failed", v.typ, typ)
	c.emitPanic(c.NewConstValue(exact.MakeString(s), types.Typ[types.String]))
	builder.SetInsertPointAtEnd(end)
	return result
}

// mustConvertE2V calls convertE2V, panicking if the assertion failed.
func (v *LLVMValue) mustConvertE2V(typ types.Type) *LLVMValue {
	result, ok := v.convertE2V(typ)

	builder := v.compiler.builder
	end := llvm.InsertBasicBlock(builder.GetInsertBlock(), "end")
	end.MoveAfter(builder.GetInsertBlock())
	failed := llvm.InsertBasicBlock(end, "failed")
	builder.CreateCondBr(ok.LLVMValue(), end, failed)
	builder.SetInsertPointAtEnd(failed)

	c := v.compiler
	s := fmt.Sprintf("convertE2V(%s, %s) failed", v.typ, typ)
	c.emitPanic(c.NewConstValue(exact.MakeString(s), types.Typ[types.String]))
	builder.SetInsertPointAtEnd(end)
	return result
}

// loadE2V loads an interface value to the specified type, without
// checking that the interface type matches.
func (v *LLVMValue) loadE2V(typ types.Type) *LLVMValue {
	c := v.compiler
	if c.types.Sizeof(typ) == 0 {
		value := llvm.ConstNull(c.types.ToLLVM(typ))
		return c.NewValue(value, typ)
	}
	if c.types.Sizeof(typ) > int64(c.target.PointerSize()) {
		ptr := c.builder.CreateExtractValue(v.LLVMValue(), 1, "")
		ptr = c.builder.CreateBitCast(ptr, llvm.PointerType(c.types.ToLLVM(typ), 0), "")
		return c.NewValue(c.builder.CreateLoad(ptr, ""), typ)
	}
	value := c.builder.CreateExtractValue(v.LLVMValue(), 1, "")
	if _, ok := typ.Underlying().(*types.Pointer); ok {
		value = c.builder.CreateBitCast(value, c.types.ToLLVM(typ), "")
		return c.NewValue(value, typ)
	}
	bits := c.target.TypeSizeInBits(c.types.ToLLVM(typ))
	value = c.builder.CreatePtrToInt(value, llvm.IntType(int(bits)), "")
	value = c.coerce(value, c.types.ToLLVM(typ))
	return c.NewValue(value, typ)
}

// interfaceTypeEquals checks, at runtime, whether an interface value
// has the specified dynamic type.
func (lhs *LLVMValue) interfaceTypeEquals(typ types.Type) *LLVMValue {
	c, b := lhs.compiler, lhs.compiler.builder
	lhsType := b.CreateExtractValue(lhs.LLVMValue(), 0, "")
	rhsType := c.types.ToRuntime(typ)
	f := c.runtime.eqtyp.LLVMValue()
	t := f.Type().ElementType().ParamTypes()[0]
	lhsType = b.CreateBitCast(lhsType, t, "")
	rhsType = b.CreateBitCast(rhsType, t, "")
	result := b.CreateCall(f, []llvm.Value{lhsType, rhsType}, "")
	return c.NewValue(result, types.Typ[types.Bool])
}
