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

	if p, fromptr := srctyp.(*types.Pointer); fromptr {
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
	lv := v.LLVMValue()
	if lv.Type().TypeKind() == llvm.PointerTypeKind {
		ptr = lv
	} else {
		// If the value fits exactly in a pointer, then we can just
		// bitcast it. Otherwise we need to malloc, and create a shim
		// function to load the receiver.
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

	runtimeType := v.compiler.types.ToRuntime(v.Type())
	runtimeType = builder.CreateBitCast(runtimeType, element_types[1], "")
	iface_struct = builder.CreateInsertValue(iface_struct, runtimeType, 1, "")

	// TODO assert either source is a named type (or pointer to), or the
	// interface has an empty methodset.

	if srcname != nil {
		// TODO check whether the functions in the struct take
		// value or pointer receivers.

		// Look up the method by name.
		// TODO check embedded types.
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
			llvm_value = builder.CreateBitCast(llvm_value, element_types[i+2], "")
			iface_struct = builder.CreateInsertValue(iface_struct, llvm_value, i+2, "")
		}
	}

	return v.compiler.NewLLVMValue(iface_struct, iface)
}

// convertI2I converts an interface to another interface.
func (v *LLVMValue) convertI2I(iface *types.Interface) (result Value, success Value) {
	c := v.compiler
	builder := v.compiler.builder
	src_typ := types.Underlying(v.Type())
	vptr := v.pointer.LLVMValue()

	//iface_struct_type := c.types.ToLLVM(iface)
	//element_types := iface_struct_type.StructElementTypes()
	//iface_elements := make([]llvm.Value, len(element_types))
	//for i, _ := range iface_elements {
	//	iface_elements[i] = llvm.ConstNull(element_types[i])
	//}
	//iface_struct := llvm.ConstStruct(iface_elements, false)
	zero_iface_struct := llvm.ConstNull(c.types.ToLLVM(iface))
	iface_struct := zero_iface_struct
	receiver := builder.CreateLoad(builder.CreateStructGEP(vptr, 0, ""), "")
	dynamicType := builder.CreateLoad(builder.CreateStructGEP(vptr, 1, ""), "")

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
			//panic("Failed to locate method: " + m.Name)
			goto check_dynamic
		} else {
			method := builder.CreateStructGEP(vptr, mi+2, "")
			fptr := builder.CreateLoad(method, "")
			iface_struct = builder.CreateInsertValue(iface_struct, fptr, i+2, "")
		}
	}
	iface_struct = builder.CreateInsertValue(iface_struct, receiver, 0, "")
	iface_struct = builder.CreateInsertValue(iface_struct, dynamicType, 1, "")
	result = c.NewLLVMValue(iface_struct, iface)
	success = ConstValue{types.Const{true}, c, types.Bool}
	return result, success

check_dynamic:
	runtimeConvertI2I := c.NamedFunction("runtime.convertI2I", "func f(typ, from, to uintptr) bool")
	llvmUintptr := runtimeConvertI2I.Type().ElementType().ParamTypes()[0]

	runtimeType := c.builder.CreatePtrToInt(c.types.ToRuntime(iface), llvmUintptr, "")
	from := c.builder.CreatePtrToInt(vptr, llvmUintptr, "")
	to := c.builder.CreateAlloca(iface_struct.Type(), "")
	c.builder.CreateStore(iface_struct, to)
	toUintptr := c.builder.CreatePtrToInt(to, llvmUintptr, "")
	args := []llvm.Value{runtimeType, from, toUintptr}
	ok := c.builder.CreateCall(runtimeConvertI2I, args, "")

	value := c.builder.CreateLoad(to, "")
	value = c.builder.CreateSelect(ok, value, zero_iface_struct, "")
	result = c.NewLLVMValue(value, iface)
	success = c.NewLLVMValue(ok, types.Bool)
	return result, success
}

// convertI2V converts an interface to a value.
func (v *LLVMValue) convertI2V(typ types.Type) (result, success Value) {
	typptrType := llvm.PointerType(llvm.Int8Type(), 0)
	runtimeType := v.compiler.types.ToRuntime(typ)
	runtimeType = llvm.ConstBitCast(runtimeType, typptrType)
	vval := v.LLVMValue()

	builder := v.compiler.builder
	ifaceType := builder.CreateExtractValue(vval, 1, "")
	diff := builder.CreatePtrDiff(runtimeType, ifaceType, "")
	zero := llvm.ConstNull(diff.Type())
	predicate := builder.CreateICmp(llvm.IntEQ, diff, zero, "")

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
	success = v.compiler.NewLLVMValue(successValue, types.Bool)

	resultValues := []llvm.Value{matchResultValue, nonmatchResultValue}
	resultBlocks := []llvm.BasicBlock{match, nonmatch}
	resultValue.AddIncoming(resultValues, resultBlocks)
	result = v.compiler.NewLLVMValue(resultValue, typ)
	return result, success
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

	value := c.builder.CreateExtractValue(v.LLVMValue(), 0, "")
	if _, ok := types.Underlying(typ).(*types.Pointer); ok {
		value = c.builder.CreateBitCast(value, c.types.ToLLVM(typ), "")
		return c.NewLLVMValue(value, typ)
	}
	bits := c.target.TypeSizeInBits(c.types.ToLLVM(typ))
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
