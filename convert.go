// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// convertI2V converts (type asserts) an interface value to a concrete type.
func (v *LLVMValue) convertI2V(typ types.Type) (result, success *LLVMValue) {
	builder := v.compiler.builder
	predicate := v.interfaceTypeEquals(typ).LLVMValue()

	// If result is zero, then we've got a match.
	end := llvm.InsertBasicBlock(builder.GetInsertBlock(), "end")
	end.MoveAfter(builder.GetInsertBlock())
	nonmatch := llvm.InsertBasicBlock(end, "nonmatch")
	match := llvm.InsertBasicBlock(nonmatch, "match")
	builder.CreateCondBr(predicate, match, nonmatch)

	builder.SetInsertPointAtEnd(match)
	matchResultValue := v.loadI2V(typ).LLVMValue()
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

// mustConvertI2V calls convertI2V, panicking if the assertion failed.
func (v *LLVMValue) mustConvertI2V(typ types.Type) *LLVMValue {
	result, ok := v.convertI2V(typ)

	builder := v.compiler.builder
	end := llvm.InsertBasicBlock(builder.GetInsertBlock(), "end")
	end.MoveAfter(builder.GetInsertBlock())
	failed := llvm.InsertBasicBlock(end, "failed")
	builder.CreateCondBr(ok.LLVMValue(), end, failed)
	builder.SetInsertPointAtEnd(failed)

	// TODO
	//s := fmt.Sprintf("convertI2V(%s, %s) failed", v.typ, typ)
	//c.visitPanic(c.NewConstValue(exact.MakeString(s), types.Typ[types.String]))
	builder.SetInsertPointAtEnd(end)
	return result
}

// loadI2V loads an interface value to the specified type, without
// checking that the interface type matches.
func (v *LLVMValue) loadI2V(typ types.Type) *LLVMValue {
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
	f := c.RuntimeFunction("runtime.eqtyp", "func(t1, t2 *rtype) bool")
	t := f.Type().ElementType().ParamTypes()[0]
	lhsType = b.CreateBitCast(lhsType, t, "")
	rhsType = b.CreateBitCast(rhsType, t, "")
	result := b.CreateCall(f, []llvm.Value{lhsType, rhsType}, "")
	return c.NewValue(result, types.Typ[types.Bool])
}
