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
	"fmt"
	"github.com/axw/gollvm/llvm"
	"./types"
	"go/ast"
	"go/token"
	"sort"
	"strconv"
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
				lv = builder.CreateBitCast(lv, llvm.IntType(int(bits)), "")
				ptr = builder.CreateIntToPtr(lv, element_types[1], "")
			} else {
				ptr = llvm.ConstNull(element_types[1])
			}
		} else {
			ptr = c.createTypeMalloc(v.compiler.types.ToLLVM(srctyp))
			builder.CreateStore(lv, ptr)
			ptr = builder.CreateBitCast(ptr, element_types[1], "")
			// TODO signal that shim functions are required. Probably later
			// we'll have the CallExpr handler pick out the type, and check
			// if the receiver is a pointer or a value type, and load as
			// necessary.
		}
	}
	runtimeType := v.compiler.types.ToRuntime(v.Type())
	runtimeType = builder.CreateBitCast(runtimeType, element_types[0], "")
	iface_struct = builder.CreateInsertValue(iface_struct, runtimeType, 0, "")
	iface_struct = builder.CreateInsertValue(iface_struct, ptr, 1, "")

	// TODO assert either source is a named type (or pointer to), or the
	// interface has an empty methodset.

	if srcname != nil {
		// TODO check whether the functions in the struct take
		// value or pointer receivers.

		// Look up the method by name.
		// TODO check embedded types.
		for i, m := range iface.Methods {
			// TODO make this loop linear by iterating through the
			// interface methods and type methods together.
			var methodobj *ast.Object
			curr := []types.Type{srcname}
			for methodobj == nil && len(curr) > 0 {
				var next []types.Type
				for _, typ := range curr {
					if p, ok := types.Underlying(typ).(*types.Pointer); ok {
						if _, ok := types.Underlying(p.Base).(*types.Struct); ok {
							typ = p.Base
						}
					}
					if n, ok := typ.(*types.Name); ok {
						methods := n.Methods
						mi := sort.Search(len(methods), func(i int) bool {
							return methods[i].Name >= m.Name
						})
						if mi < len(methods) && methods[mi].Name == m.Name {
							methodobj = methods[mi]
							break
						}
					}
					if typ, ok := types.Underlying(typ).(*types.Struct); ok {
						for _, field := range typ.Fields {
							if field.Name == "" {
								typ := field.Type.(types.Type)
								next = append(next, typ)
							}
						}
					}
				}
				curr = next
			}
			if methodobj == nil {
				msg := fmt.Sprintf("Failed to locate (%s).%s", srcname.Obj.Name, m.Name)
				panic(msg)
			}
			method := v.compiler.Resolve(methodobj).(*LLVMValue)
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
	val := v.LLVMValue()

	zero_iface_struct := llvm.ConstNull(c.types.ToLLVM(iface))
	iface_struct := zero_iface_struct
	dynamicType := builder.CreateExtractValue(val, 0, "")
	receiver := builder.CreateExtractValue(val, 1, "")

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
			fptr := builder.CreateExtractValue(val, mi+2, "")
			iface_struct = builder.CreateInsertValue(iface_struct, fptr, i+2, "")
		}
	}
	iface_struct = builder.CreateInsertValue(iface_struct, dynamicType, 0, "")
	iface_struct = builder.CreateInsertValue(iface_struct, receiver, 1, "")
	result = c.NewLLVMValue(iface_struct, iface)
	success = ConstValue{types.Const{true}, c, types.Bool}
	return result, success

check_dynamic:
	runtimeConvertI2I := c.NamedFunction("runtime.convertI2I", "func f(typ, from, to uintptr) bool")
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
	result = c.NewLLVMValue(value, iface)
	success = c.NewLLVMValue(ok, types.Bool)
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
	c.visitPanic(c.NewConstValue(token.STRING, strconv.Quote(s)))
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
	success = v.compiler.NewLLVMValue(successValue, types.Bool)

	resultValues := []llvm.Value{matchResultValue, nonmatchResultValue}
	resultBlocks := []llvm.BasicBlock{match, nonmatch}
	resultValue.AddIncoming(resultValues, resultBlocks)
	result = v.compiler.NewLLVMValue(resultValue, typ)
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
	c.visitPanic(c.NewConstValue(token.STRING, strconv.Quote(s)))
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
	default:
		return c.builder.CreateBitCast(v, t, "")
	}
	panic("unreachable")
}

// loadI2V loads an interface value to a type, without checking
// that the interface type matches.
func (v *LLVMValue) loadI2V(typ types.Type) Value {
	c := v.compiler
	if c.sizeofType(typ) > c.target.PointerSize() {
		ptr := c.builder.CreateExtractValue(v.LLVMValue(), 1, "")
		typ = &types.Pointer{Base: typ}
		ptr = c.builder.CreateBitCast(ptr, c.types.ToLLVM(typ), "")
		return c.NewLLVMValue(ptr, typ).makePointee()
	}

	value := c.builder.CreateExtractValue(v.LLVMValue(), 1, "")
	if _, ok := types.Underlying(typ).(*types.Pointer); ok {
		value = c.builder.CreateBitCast(value, c.types.ToLLVM(typ), "")
		return c.NewLLVMValue(value, typ)
	}
	bits := c.target.TypeSizeInBits(c.types.ToLLVM(typ))
	value = c.builder.CreatePtrToInt(value, llvm.IntType(int(bits)), "")
	value = c.coerce(value, c.types.ToLLVM(typ))
	return c.NewLLVMValue(value, typ)
}

func (lhs *LLVMValue) interfaceTypeEquals(typ types.Type) *LLVMValue {
	c, b := lhs.compiler, lhs.compiler.builder
	lhsType := b.CreateExtractValue(lhs.LLVMValue(), 0, "")
	rhsType := c.types.ToRuntime(typ)
	f := c.NamedFunction("runtime.eqtyp", "func f(t1, t2 *type_) bool")
	t := f.Type().ElementType().ParamTypes()[0]
	lhsType = b.CreateBitCast(lhsType, t, "")
	rhsType = b.CreateBitCast(rhsType, t, "")
	result := b.CreateCall(f, []llvm.Value{lhsType, rhsType}, "")
	return c.NewLLVMValue(result, types.Bool)
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

	f := c.NamedFunction("runtime.compareI2I", "func f(t1, t2, v1, v2 uintptr) bool")
	result := c.builder.CreateCall(f, args, "")
	return c.NewLLVMValue(result, types.Bool)
}

// vim: set ft=go :
