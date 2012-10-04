/*
Copyright (c) 2012 Andrew Wilkins <axwalk@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package llgo

import (
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo/types"
	"sort"
)

// convertV2I converts a value to an interface.
func (v *LLVMValue) convertV2I(iface *types.Interface) Value {
	// TODO deref indirect value, then use 'pointer' as pointer value.
	var srcname *types.Name
	srctyp := v.Type()
	if name, isname := srctyp.(*types.Name); isname {
		srcname = name
		srctyp = name.Underlying
	}

	isptr := false
	if p, fromptr := srctyp.(*types.Pointer); fromptr {
		isptr = true
		srctyp = p.Base
		if name, isname := srctyp.(*types.Name); isname {
			srcname = name
			srctyp = name.Underlying
		}
	}

	iface_struct_type := v.compiler.types.ToLLVM(iface)
	element_types := iface_struct_type.StructElementTypes()
	iface_elements := make([]llvm.Value, len(element_types))
	for i, _ := range iface_elements {
		iface_elements[i] = llvm.ConstNull(element_types[i])
	}
	iface_struct := llvm.ConstStruct(iface_elements, false)

	builder := v.compiler.builder
	var ptr llvm.Value
	if isptr {
		ptr = v.LLVMValue()
	} else {
		// If the value fits exactly in a pointer, then we can just
		// bitcast it. Otherwise we need to malloc, and create a shim
		// function to load the receiver.
		lv := v.LLVMValue()
		c := v.compiler
		ptrsize := c.target.PointerSize()
		if c.target.TypeStoreSize(lv.Type()) <= uint64(ptrsize) {
			bits := c.target.TypeSizeInBits(lv.Type())
			if bits > 0 {
				lv = builder.CreateBitCast(lv, llvm.IntType(int(bits)), "")
				ptr = builder.CreateIntToPtr(lv, element_types[0], "")
			} else {
				ptr = llvm.ConstNull(element_types[0])
			}
		} else {
			ptr = builder.CreateMalloc(v.compiler.types.ToLLVM(srctyp), "")
			builder.CreateStore(lv, ptr)
			// TODO signal that shim functions are required. Probably later
			// we'll have the CallExpr handler pick out the type, and check
			// if the receiver is a pointer or a value type, and load as
			// necessary.
		}
	}
	ptr = builder.CreateBitCast(ptr, element_types[0], "")
	iface_struct = builder.CreateInsertValue(iface_struct, ptr, 0, "")

	var runtimeType llvm.Value
	if srcname != nil {
		runtimeType = v.compiler.types.ToRuntime(srcname)
	} else {
		runtimeType = v.compiler.types.ToRuntime(srctyp)
	}
	runtimeType = builder.CreateBitCast(runtimeType, element_types[1], "")
	iface_struct = builder.CreateInsertValue(iface_struct, runtimeType, 1, "")

	// TODO assert either source is a named type (or pointer to), or the
	// interface has an empty methodset.

	if srcname != nil {
		// TODO check whether the functions in the struct take
		// value or pointer receivers.

		// Look up the method by name.
		methods := srcname.Methods
		for i, m := range iface.Methods {
			// TODO make this loop linear by iterating through the
			// interface methods and type methods together.
			mi := sort.Search(len(methods), func(i int) bool {
				return methods[i].Name >= m.Name
			})
			if mi >= len(methods) || methods[mi].Name != m.Name {
				panic("Failed to locate method: " + m.Name)
			}
			method_obj := methods[mi]
			method := v.compiler.Resolve(method_obj).(*LLVMValue)
			llvm_value := method.LLVMValue()
			llvm_value = builder.CreateBitCast(
				llvm_value, element_types[i+2], "")
			iface_struct = builder.CreateInsertValue(
				iface_struct, llvm_value, i+2, "")
		}
	}

	return v.compiler.NewLLVMValue(iface_struct, iface)
}

// convertI2I converts an interface to another interface.
func (v *LLVMValue) convertI2I(iface *types.Interface) Value {
	builder := v.compiler.builder
	src_typ := v.Type()
	vptr := v.pointer.LLVMValue()
	src_typ = src_typ.(*types.Name).Underlying

	iface_struct_type := v.compiler.types.ToLLVM(iface)
	element_types := iface_struct_type.StructElementTypes()
	iface_elements := make([]llvm.Value, len(element_types))
	for i, _ := range iface_elements {
		iface_elements[i] = llvm.ConstNull(element_types[i])
	}
	iface_struct := llvm.ConstStruct(iface_elements, false)
	receiver := builder.CreateLoad(builder.CreateStructGEP(vptr, 0, ""), "")
	iface_struct = builder.CreateInsertValue(iface_struct, receiver, 0, "")

	// TODO check whether the functions in the struct take
	// value or pointer receivers.

	// TODO handle dynamic interface conversion (non-subset).
	methods := src_typ.(*types.Interface).Methods
	for i, m := range iface.Methods {
		// TODO make this loop linear by iterating through the
		// interface methods and type methods together.
		mi := sort.Search(len(methods), func(i int) bool {
			return methods[i].Name >= m.Name
		})
		if mi >= len(methods) || methods[mi].Name != m.Name {
			panic("Failed to locate method: " + m.Name)
		}
		method := builder.CreateStructGEP(vptr, mi+2, "")
		iface_struct = builder.CreateInsertValue(
			iface_struct, builder.CreateLoad(method, ""), i+2, "")
	}
	return v.compiler.NewLLVMValue(iface_struct, iface)
}

// convertI2V converts an interface to a value.
func (v *LLVMValue) convertI2V(typ types.Type) Value {
	typptrType := llvm.PointerType(llvm.Int8Type(), 0)
	runtimeType := v.compiler.types.ToRuntime(typ)
	runtimeType = llvm.ConstBitCast(runtimeType, typptrType)
	vptr := v.pointer.LLVMValue()

	builder := v.compiler.builder
	ifaceType := builder.CreateLoad(builder.CreateStructGEP(vptr, 1, ""), "")
	diff := builder.CreatePtrDiff(runtimeType, ifaceType, "")
	zero := llvm.ConstNull(diff.Type())
	predicate := builder.CreateICmp(llvm.IntEQ, diff, zero, "")
	llvmtype := v.compiler.types.ToLLVM(typ)
	result := builder.CreateAlloca(llvmtype, "")

	// If result is zero, then we've got a match.
	end := llvm.InsertBasicBlock(builder.GetInsertBlock(), "end")
	end.MoveAfter(builder.GetInsertBlock())
	nonmatch := llvm.InsertBasicBlock(end, "nonmatch")
	match := llvm.InsertBasicBlock(nonmatch, "match")
	builder.CreateCondBr(predicate, match, nonmatch)

	builder.SetInsertPointAtEnd(match)
	value := builder.CreateLoad(builder.CreateStructGEP(vptr, 0, ""), "")
	value = builder.CreateBitCast(value, result.Type(), "")
	value = builder.CreateLoad(value, "")
	builder.CreateStore(value, result)
	builder.CreateBr(end)

	// TODO should return {value, ok}
	builder.SetInsertPointAtEnd(nonmatch)
	builder.CreateStore(llvm.ConstNull(llvmtype), result)
	builder.CreateBr(end)

	//builder.SetInsertPointAtEnd(end)
	result = builder.CreateLoad(result, "")
	return v.compiler.NewLLVMValue(result, typ)
}

// loadI2V loads an interface value to a type, without checking
// that the interface type matches.
func (v *LLVMValue) loadI2V(typ types.Type) Value {
	c := v.compiler
	if c.sizeofType(typ) > c.target.PointerSize() {
		ptr := c.builder.CreateExtractValue(v.LLVMValue(), 0, "")
		typ = &types.Pointer{Base: typ}
		ptr = c.builder.CreateBitCast(ptr, c.types.ToLLVM(typ), "")
		return c.NewLLVMValue(ptr, typ).makePointee()
	}
	bits := c.target.TypeSizeInBits(c.types.ToLLVM(typ))
	value := c.builder.CreateExtractValue(v.LLVMValue(), 0, "")
	value = c.builder.CreatePtrToInt(value, llvm.IntType(int(bits)), "")
	value = c.builder.CreateBitCast(value, c.types.ToLLVM(typ), "")
	return c.NewLLVMValue(value, typ)
}

// interfacesEqual compares two interfaces for equality, returning
// a dynamic boolean value.
func (lhs *LLVMValue) compareI2I(rhs *LLVMValue) Value {
	c := lhs.compiler
	b := c.builder

	lhsValue := b.CreateExtractValue(lhs.LLVMValue(), 0, "")
	rhsValue := b.CreateExtractValue(rhs.LLVMValue(), 0, "")
	lhsType := b.CreateExtractValue(lhs.LLVMValue(), 1, "")
	rhsType := b.CreateExtractValue(rhs.LLVMValue(), 1, "")

	llvmUintptr := c.target.IntPtrType()
	runtimeCompareI2I := c.module.Module.NamedFunction("runtime.compareI2I")
	if runtimeCompareI2I.IsNil() {
		args := []llvm.Type{llvmUintptr, llvmUintptr, llvmUintptr, llvmUintptr}
		functype := llvm.FunctionType(llvm.Int1Type(), args, false)
		runtimeCompareI2I = llvm.AddFunction(
			c.module.Module, "runtime.compareI2I", functype)
	}

	args := []llvm.Value{
		c.builder.CreatePtrToInt(lhsType, llvmUintptr, ""),
		c.builder.CreatePtrToInt(rhsType, llvmUintptr, ""),
		c.builder.CreatePtrToInt(lhsValue, llvmUintptr, ""),
		c.builder.CreatePtrToInt(rhsValue, llvmUintptr, ""),
	}

	result := c.builder.CreateCall(runtimeCompareI2I, args, "")
	return c.NewLLVMValue(result, types.Bool)
}

// vim: set ft=go :
