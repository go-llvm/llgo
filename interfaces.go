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

// ConvertV2I converts a value to an interface.
func (v *LLVMValue) convertV2I(iface *types.Interface) Value {
	// TODO deref indirect value, then use 'address' as pointer
	// value.
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

		// FIXME this is obviously not good enough. We'll need to generate
		// code here to detect whether the size of the value (possibly non
		// portable) fits into a pointer (definitely non portable).
		if types.Identical(types.Underlying(srctyp), types.Int) {
			ptr = builder.CreateIntToPtr(v.LLVMValue(), element_types[0], "")
		} else {
			ptr = builder.CreateMalloc(v.compiler.types.ToLLVM(srctyp), "")
			builder.CreateStore(v.LLVMValue(), ptr)
			// TODO signal that shim functions are required. Probably later
			// we'll have the CallExpr handler pick out the type, and check
			// if the receiver is a pointer or a value type, and load as
			// necessary.
		}
	}
	ptr = builder.CreateBitCast(ptr, element_types[0], "")
	iface_struct = builder.CreateInsertValue(iface_struct, ptr, 0, "")

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
				llvm_value,
				element_types[i+1], "")
			iface_struct = builder.CreateInsertValue(
				iface_struct, llvm_value,
				i+1, "")
		}
	}

	return v.compiler.NewLLVMValue(iface_struct, iface)
}

// ConvertI2I converts an interface to another interface.
func (v *LLVMValue) convertI2I(iface *types.Interface) Value {
	builder := v.compiler.builder
	src_typ := v.typ
	var vptr llvm.Value
	if v.indirect {
		src_typ = types.Deref(src_typ)
		vptr = v.LLVMValue()
	} else {
		vptr = v.address.LLVMValue()
	}
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
		method := builder.CreateStructGEP(vptr, mi+1, "")
		iface_struct = builder.CreateInsertValue(
			iface_struct, builder.CreateLoad(method, ""), i+1, "")
	}
	return v.compiler.NewLLVMValue(iface_struct, iface)
}

// vim: set ft=go :
