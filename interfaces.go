// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/exact"
	"code.google.com/p/go.tools/go/types"
	"fmt"
	"github.com/axw/gollvm/llvm"
	"go/token"
)

// convertV2I converts a value to an interface.
func (v *LLVMValue) convertV2I(iface *types.Interface) *LLVMValue {
	var srcname *types.Named
	srctyp := v.Type()
	if name, isname := srctyp.(*types.Named); isname {
		srcname = name
		srctyp = name.Underlying()
	}

	var isptr bool
	if p, fromptr := srctyp.(*types.Pointer); fromptr {
		isptr = true
		srctyp = p.Elem()
		if name, isname := srctyp.(*types.Named); isname {
			srcname = name
			srctyp = name.Underlying()
		}
	}

	iface_struct_type := v.compiler.types.ToLLVM(iface)
	element_types := iface_struct_type.StructElementTypes()
	zero_iface_struct := llvm.ConstNull(iface_struct_type)
	iface_struct := zero_iface_struct

	builder := v.compiler.builder
	var ptr llvm.Value
	lv := v.LLVMValue()
	if lv.Type().TypeKind() == llvm.PointerTypeKind {
		ptr = builder.CreateBitCast(lv, element_types[1], "")
	} else {
		// If the value fits exactly in a pointer, then we can just
		// bitcast it. Otherwise we need to malloc, and create a shim
		// function to load the receiver.
		c := v.compiler
		ptrsize := c.target.PointerSize()
		if c.target.TypeStoreSize(lv.Type()) <= uint64(ptrsize) {
			bits := c.target.TypeSizeInBits(lv.Type())
			if bits > 0 {
				lv = c.coerce(lv, llvm.IntType(int(bits)))
				ptr = builder.CreateIntToPtr(lv, element_types[1], "")
			} else {
				ptr = llvm.ConstNull(element_types[1])
			}
		} else {
			if lv.IsConstant() {
				ptr = llvm.AddGlobal(c.module.Module, lv.Type(), "")
				ptr.SetInitializer(lv)
			} else {
				ptr = c.createTypeMalloc(lv.Type())
				builder.CreateStore(lv, ptr)
			}
			ptr = builder.CreateBitCast(ptr, element_types[1], "")
			isptr = true
		}
	}
	runtimeType := v.compiler.types.ToRuntime(v.Type())
	runtimeType = builder.CreateBitCast(runtimeType, element_types[0], "")
	iface_struct = builder.CreateInsertValue(iface_struct, runtimeType, 0, "")
	iface_struct = builder.CreateInsertValue(iface_struct, ptr, 1, "")

	if srcname != nil {
		// Look up the method by name.
		for i, m := range sortedMethods(iface) {
			method := v.compiler.methods(srcname).lookup(m.Name(), isptr)
			methodident := v.compiler.objectdata[method].Ident
			llvm_value := v.compiler.Resolve(methodident).LLVMValue()
			llvm_value = builder.CreateExtractValue(llvm_value, 0, "")
			llvm_value = builder.CreateBitCast(llvm_value, element_types[i+2], "")
			iface_struct = builder.CreateInsertValue(iface_struct, llvm_value, i+2, "")
		}
	}

	return v.compiler.NewValue(iface_struct, iface)
}

// convertI2I converts an interface to another interface.
func (v *LLVMValue) convertI2I(iface *types.Interface) (result *LLVMValue, success *LLVMValue) {
	c := v.compiler
	builder := v.compiler.builder
	src_typ := v.Type().Underlying()
	val := v.LLVMValue()

	zero_iface_struct := llvm.ConstNull(c.types.ToLLVM(iface))
	iface_struct := zero_iface_struct
	dynamicType := builder.CreateExtractValue(val, 0, "")
	receiver := builder.CreateExtractValue(val, 1, "")

	// TODO check whether the functions in the struct take
	// value or pointer receivers.

	// TODO handle dynamic interface conversion (non-subset).
	srciface := src_typ.(*types.Interface)
	for i, m := range sortedMethods(iface) {
		// FIXME make loop linear.
		var mi int
		for _, m2 := range sortedMethods(srciface) {
			if m2.Name() == m.Name() {
				break
			}
			mi++
		}
		if mi >= srciface.NumMethods() {
			goto check_dynamic
		} else {
			fptr := builder.CreateExtractValue(val, mi+2, "")
			iface_struct = builder.CreateInsertValue(iface_struct, fptr, i+2, "")
		}
	}
	iface_struct = builder.CreateInsertValue(iface_struct, dynamicType, 0, "")
	iface_struct = builder.CreateInsertValue(iface_struct, receiver, 1, "")
	result = c.NewValue(iface_struct, iface)
	success = c.NewValue(llvm.ConstAllOnes(llvm.Int1Type()), types.Typ[types.Bool])
	return result, success

check_dynamic:
	runtimeConvertI2I := c.NamedFunction("runtime.convertI2I", "func(typ, from, to uintptr) bool")
	llvmUintptr := runtimeConvertI2I.Type().ElementType().ParamTypes()[0]

	var vptr llvm.Value
	if v.pointer != nil {
		vptr = v.pointer.LLVMValue()
	} else {
		vptr = c.builder.CreateAlloca(val.Type(), "")
		c.builder.CreateStore(val, vptr)
	}

	runtimeType := c.builder.CreatePtrToInt(c.types.ToRuntime(iface), llvmUintptr, "")
	from := c.builder.CreatePtrToInt(vptr, llvmUintptr, "")
	to := c.builder.CreateAlloca(iface_struct.Type(), "")
	c.builder.CreateStore(iface_struct, to)
	toUintptr := c.builder.CreatePtrToInt(to, llvmUintptr, "")
	args := []llvm.Value{runtimeType, from, toUintptr}
	ok := c.builder.CreateCall(runtimeConvertI2I, args, "")

	value := c.builder.CreateLoad(to, "")
	value = c.builder.CreateSelect(ok, value, zero_iface_struct, "")
	result = c.NewValue(value, iface)
	success = c.NewValue(ok, types.Typ[types.Bool])
	return result, success
}

func (v *LLVMValue) mustConvertI2I(iface *types.Interface) Value {
	result, ok := v.convertI2I(iface)

	c, builder := v.compiler, v.compiler.builder
	end := llvm.InsertBasicBlock(builder.GetInsertBlock(), "end")
	end.MoveAfter(builder.GetInsertBlock())
	failed := llvm.InsertBasicBlock(end, "failed")
	builder.CreateCondBr(ok.LLVMValue(), end, failed)
	builder.SetInsertPointAtEnd(failed)

	s := fmt.Sprintf("convertI2I(%s, %s) failed", v.typ, iface)
	c.visitPanic(c.NewConstValue(exact.MakeString(s), types.Typ[types.String]))
	builder.SetInsertPointAtEnd(end)
	return result
}

// convertI2V converts an interface to a value.
func (v *LLVMValue) convertI2V(typ types.Type) (result, success Value) {
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

func (v *LLVMValue) mustConvertI2V(typ types.Type) Value {
	result, ok := v.convertI2V(typ)

	c, builder := v.compiler, v.compiler.builder
	end := llvm.InsertBasicBlock(builder.GetInsertBlock(), "end")
	end.MoveAfter(builder.GetInsertBlock())
	failed := llvm.InsertBasicBlock(end, "failed")
	builder.CreateCondBr(ok.LLVMValue(), end, failed)
	builder.SetInsertPointAtEnd(failed)

	s := fmt.Sprintf("convertI2V(%s, %s) failed", v.typ, typ)
	c.visitPanic(c.NewConstValue(exact.MakeString(s), types.Typ[types.String]))
	builder.SetInsertPointAtEnd(end)
	return result
}

// coerce yields a value of the the type specified, initialised
// to the exact bit pattern as in the specified value.
//
// Note: the specified value must be a non-aggregate, and its type
// and the specified type must have the same size.
func (c *compiler) coerce(v llvm.Value, t llvm.Type) llvm.Value {
	switch t.TypeKind() {
	case llvm.ArrayTypeKind, llvm.StructTypeKind:
		ptr := c.builder.CreateAlloca(t, "")
		ptrv := c.builder.CreateBitCast(ptr, llvm.PointerType(v.Type(), 0), "")
		c.builder.CreateStore(v, ptrv)
		return c.builder.CreateLoad(ptr, "")
	}

	vt := v.Type()
	switch vt.TypeKind() {
	case llvm.ArrayTypeKind, llvm.StructTypeKind:
		ptr := c.builder.CreateAlloca(vt, "")
		c.builder.CreateStore(v, ptr)
		ptrt := c.builder.CreateBitCast(ptr, llvm.PointerType(t, 0), "")
		return c.builder.CreateLoad(ptrt, "")
	}

	return c.builder.CreateBitCast(v, t, "")
}

// loadI2V loads an interface value to a type, without checking
// that the interface type matches.
func (v *LLVMValue) loadI2V(typ types.Type) *LLVMValue {
	c := v.compiler
	if c.types.Sizeof(typ) > int64(c.target.PointerSize()) {
		ptr := c.builder.CreateExtractValue(v.LLVMValue(), 1, "")
		typ = types.NewPointer(typ)
		ptr = c.builder.CreateBitCast(ptr, c.types.ToLLVM(typ), "")
		return c.NewValue(ptr, typ).makePointee()
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

func (lhs *LLVMValue) interfaceTypeEquals(typ types.Type) *LLVMValue {
	c, b := lhs.compiler, lhs.compiler.builder
	lhsType := b.CreateExtractValue(lhs.LLVMValue(), 0, "")
	rhsType := c.types.ToRuntime(typ)
	f := c.NamedFunction("runtime.eqtyp", "func(t1, t2 *rtype) bool")
	t := f.Type().ElementType().ParamTypes()[0]
	lhsType = b.CreateBitCast(lhsType, t, "")
	rhsType = b.CreateBitCast(rhsType, t, "")
	result := b.CreateCall(f, []llvm.Value{lhsType, rhsType}, "")
	return c.NewValue(result, types.Typ[types.Bool])
}

// interfacesEqual compares two interfaces for equality, returning
// a dynamic boolean value.
func (lhs *LLVMValue) compareI2I(rhs *LLVMValue) Value {
	c := lhs.compiler
	b := c.builder

	lhsType := b.CreateExtractValue(lhs.LLVMValue(), 0, "")
	rhsType := b.CreateExtractValue(rhs.LLVMValue(), 0, "")
	lhsValue := b.CreateExtractValue(lhs.LLVMValue(), 1, "")
	rhsValue := b.CreateExtractValue(rhs.LLVMValue(), 1, "")

	llvmUintptr := c.target.IntPtrType()
	args := []llvm.Value{
		c.builder.CreatePtrToInt(lhsType, llvmUintptr, ""),
		c.builder.CreatePtrToInt(rhsType, llvmUintptr, ""),
		c.builder.CreatePtrToInt(lhsValue, llvmUintptr, ""),
		c.builder.CreatePtrToInt(rhsValue, llvmUintptr, ""),
	}

	f := c.NamedFunction("runtime.compareI2I", "func(t1, t2, v1, v2 uintptr) bool")
	result := c.builder.CreateCall(f, args, "")
	return c.NewValue(result, types.Typ[types.Bool])
}

func (lhs *LLVMValue) compareI2V(rhs *LLVMValue) Value {
	c := lhs.compiler
	predicate := lhs.interfaceTypeEquals(rhs.typ).LLVMValue()

	end := llvm.InsertBasicBlock(c.builder.GetInsertBlock(), "end")
	end.MoveAfter(c.builder.GetInsertBlock())
	nonmatch := llvm.InsertBasicBlock(end, "nonmatch")
	match := llvm.InsertBasicBlock(nonmatch, "match")
	c.builder.CreateCondBr(predicate, match, nonmatch)

	c.builder.SetInsertPointAtEnd(match)
	lhsValue := lhs.loadI2V(rhs.typ)
	matchResultValue := lhsValue.BinaryOp(token.EQL, rhs).LLVMValue()
	c.builder.CreateBr(end)

	c.builder.SetInsertPointAtEnd(nonmatch)
	nonmatchResultValue := llvm.ConstNull(llvm.Int1Type())
	c.builder.CreateBr(end)

	c.builder.SetInsertPointAtEnd(end)
	resultValue := c.builder.CreatePHI(matchResultValue.Type(), "")
	resultValues := []llvm.Value{matchResultValue, nonmatchResultValue}
	resultBlocks := []llvm.BasicBlock{match, nonmatch}
	resultValue.AddIncoming(resultValues, resultBlocks)
	return c.NewValue(resultValue, types.Typ[types.Bool])
}

// vim: set ft=go :
