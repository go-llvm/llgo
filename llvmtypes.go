/*
Copyright (c) 2011, 2012 Andrew Wilkins <axwalk@gmail.com>

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
	"github.com/axw/llgo/types"
	"go/ast"
	"reflect"
)

var (
	runtimeCommonType,
	runtimeUncommonType,
	runtimeArrayType,
	runtimeChanType,
	runtimeFuncType,
	runtimeInterfaceType,
	runtimeMapType,
	runtimePtrType,
	runtimeSliceType,
	runtimeStructType llvm.Type
)

type TypeMap struct {
	module  llvm.Module
	types   map[types.Type]llvm.Type  // compile-time LLVM type
	runtime map[types.Type]llvm.Value // runtime/reflect type representation
}

func NewTypeMap(module llvm.Module) *TypeMap {
	tm := &TypeMap{module: module}
	tm.types = make(map[types.Type]llvm.Type)
	tm.runtime = make(map[types.Type]llvm.Value)

	// Load "reflect.go", and generate LLVM types for the runtime type
	// structures.
	pkg, err := parseReflect()
	if err != nil {
		panic(err) // FIXME return err
	}
	objToLLVMType := func(name string) llvm.Type {
		obj := pkg.Scope.Lookup(name)
		return tm.ToLLVM(obj.Type.(types.Type))
	}
	runtimeCommonType = objToLLVMType("commonType")
	runtimeUncommonType = objToLLVMType("uncommonType")
	runtimeArrayType = objToLLVMType("arrayType")
	runtimeChanType = objToLLVMType("chanType")
	runtimeFuncType = objToLLVMType("funcType")
	runtimeInterfaceType = objToLLVMType("interfaceType")
	runtimeMapType = objToLLVMType("mapType")
	runtimePtrType = objToLLVMType("ptrType")
	runtimeSliceType = objToLLVMType("sliceType")
	runtimeStructType = objToLLVMType("structType")

	return tm
}

func (tm *TypeMap) ToLLVM(t types.Type) llvm.Type {
	t = types.Underlying(t)
	lt, ok := tm.types[t]
	if !ok {
		lt = tm.makeLLVMType(t)
		if lt.IsNil() {
			panic(fmt.Sprint("Failed to create LLVM type for: ", t))
		}
		tm.types[t] = lt
	}
	return lt
}

func (tm *TypeMap) ToRuntime(t types.Type) llvm.Value {
	t = types.Underlying(t)
	r, ok := tm.runtime[t]
	if !ok {
		r = tm.makeRuntimeType(t)
		if r.IsNil() {
			panic(fmt.Sprint("Failed to create runtime type for: ", t))
		}
		tm.runtime[t] = r
	}
	return r
}

func (tm *TypeMap) makeLLVMType(t types.Type) llvm.Type {
	switch t := t.(type) {
	case *types.Bad:
		return tm.badLLVMType(t)
	case *types.Basic:
		return tm.basicLLVMType(t)
	case *types.Array:
		return tm.arrayLLVMType(t)
	case *types.Slice:
		return tm.sliceLLVMType(t)
	case *types.Struct:
		return tm.structLLVMType(t)
	case *types.Pointer:
		return tm.pointerLLVMType(t)
	case *types.Func:
		return tm.funcLLVMType(t)
	case *types.Interface:
		return tm.interfaceLLVMType(t)
	case *types.Map:
		return tm.mapLLVMType(t)
	case *types.Chan:
		return tm.chanLLVMType(t)
	case *types.Name:
		return tm.nameLLVMType(t)
	}
	panic("unreachable")
}

func (tm *TypeMap) badLLVMType(b *types.Bad) llvm.Type {
	return llvm.Type{nil}
}

func (tm *TypeMap) basicLLVMType(b *types.Basic) llvm.Type {
	switch b.Kind {
	case types.BoolKind:
		return llvm.Int1Type()
	case types.ByteKind, types.Int8Kind, types.Uint8Kind:
		return llvm.Int8Type()
	case types.Int16Kind, types.Uint16Kind:
		return llvm.Int16Type()
	case types.UnsafePointerKind, types.UintptrKind, types.Int32Kind, types.Uint32Kind: // XXX uintptr size depends on bit width
		return llvm.Int32Type()
	case types.Int64Kind, types.Uint64Kind:
		return llvm.Int64Type()
	case types.Float32Kind:
		return llvm.FloatType()
	case types.Float64Kind:
		return llvm.DoubleType()
	//case Complex64: TODO
	//case Complex128:
	//case UntypedInt:
	//case UntypedFloat:
	//case UntypedComplex:
	case types.StringKind:
		i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
		elements := []llvm.Type{i8ptr, llvm.Int32Type()}
		return llvm.StructType(elements, false)
	case types.RuneKind:
		return tm.basicLLVMType(types.Int.Underlying.(*types.Basic))
	}
	panic(fmt.Sprint("unhandled kind: ", b.Kind))
}

func (tm *TypeMap) arrayLLVMType(a *types.Array) llvm.Type {
	return llvm.ArrayType(tm.ToLLVM(a.Elt), int(a.Len))
}

func (tm *TypeMap) sliceLLVMType(s *types.Slice) llvm.Type {
	elements := []llvm.Type{
		llvm.PointerType(tm.ToLLVM(s.Elt), 0),
		llvm.Int32Type(),
		llvm.Int32Type(),
	}
	return llvm.StructType(elements, false)
}

func (tm *TypeMap) structLLVMType(s *types.Struct) llvm.Type {
	// Types may be circular, so we need to first create an empty
	// struct type, then fill in its body after visiting its
	// members.
	typ, ok := tm.types[s]
	if !ok {
		typ = llvm.GlobalContext().StructCreateNamed("")
		tm.types[s] = typ
		elements := make([]llvm.Type, len(s.Fields))
		for i, f := range s.Fields {
			ft := f.Type.(types.Type)
			elements[i] = tm.ToLLVM(ft)
		}
		typ.StructSetBody(elements, false)
	}
	return typ
}

func (tm *TypeMap) pointerLLVMType(p *types.Pointer) llvm.Type {
	return llvm.PointerType(tm.ToLLVM(p.Base), 0)
}

func (tm *TypeMap) funcLLVMType(f *types.Func) llvm.Type {
	param_types := make([]llvm.Type, 0)

	// Add receiver parameter.
	if f.Recv != nil {
		recv_type := f.Recv.Type.(types.Type)
		param_types = append(param_types, tm.ToLLVM(recv_type))
	}

	for i, param := range f.Params {
		param_type := param.Type.(types.Type)
		if f.IsVariadic && i == len(f.Params)-1 {
			param_type = &types.Slice{Elt: param_type}
		}
		param_types = append(param_types, tm.ToLLVM(param_type))
	}

	var return_type llvm.Type
	switch len(f.Results) {
	case 0:
		return_type = llvm.VoidType()
	case 1:
		return_type = tm.ToLLVM(f.Results[0].Type.(types.Type))
	default:
		elements := make([]llvm.Type, len(f.Results))
		for i, result := range f.Results {
			elements[i] = tm.ToLLVM(result.Type.(types.Type))
		}
		return_type = llvm.StructType(elements, false)
	}

	fn_type := llvm.FunctionType(return_type, param_types, false)
	return llvm.PointerType(fn_type, 0)
}

func (tm *TypeMap) interfaceLLVMType(i *types.Interface) llvm.Type {
	valptr_type := llvm.PointerType(llvm.Int8Type(), 0)
	typptr_type := valptr_type // runtimeCommonType may not be defined yet
	elements := make([]llvm.Type, 2+len(i.Methods))
	elements[0] = valptr_type // value
	elements[1] = typptr_type // type
	for n, m := range i.Methods {
		// Add an opaque pointer parameter to the function for the
		// struct pointer.
		fntype := m.Type.(*types.Func)
		receiver_type := &types.Pointer{Base: types.Int8}
		fntype.Recv = ast.NewObj(ast.Var, "")
		fntype.Recv.Type = receiver_type
		fnptr := types.Pointer{Base: fntype}
		elements[n+2] = tm.ToLLVM(&fnptr)
	}
	return llvm.StructType(elements, false)
}

func (tm *TypeMap) mapLLVMType(m *types.Map) llvm.Type {
	// XXX This map type will change in the future, when I get around to it.
	// At the moment, it's representing a really dumb singly linked list.
	list_type := llvm.GlobalContext().StructCreateNamed("")
	list_ptr_type := llvm.PointerType(list_type, 0)
	size_type := llvm.Int32Type()
	element_types := []llvm.Type{size_type, list_type}
	typ := llvm.StructType(element_types, false)
	tm.types[m] = typ

	list_element_types := []llvm.Type{
		list_ptr_type, tm.ToLLVM(m.Key), tm.ToLLVM(m.Elt)}
	list_type.StructSetBody(list_element_types, false)
	return typ
}

func (tm *TypeMap) chanLLVMType(c *types.Chan) llvm.Type {
	panic("unimplemented")
}

func (tm *TypeMap) nameLLVMType(n *types.Name) llvm.Type {
	return tm.ToLLVM(n.Underlying)
}

///////////////////////////////////////////////////////////////////////////////

func (tm *TypeMap) makeRuntimeType(t types.Type) llvm.Value {
	switch t := t.(type) {
	case *types.Bad:
		return tm.badRuntimeType(t)
	case *types.Basic:
		return tm.basicRuntimeType(t)
	case *types.Array:
		return tm.arrayRuntimeType(t)
	case *types.Slice:
		return tm.sliceRuntimeType(t)
	case *types.Struct:
		return tm.structRuntimeType(t)
	case *types.Pointer:
		return tm.pointerRuntimeType(t)
	case *types.Func:
		return tm.funcRuntimeType(t)
	case *types.Interface:
		return tm.interfaceRuntimeType(t)
	case *types.Map:
		return tm.mapRuntimeType(t)
	case *types.Chan:
		return tm.chanRuntimeType(t)
	case *types.Name:
		return tm.nameRuntimeType(t)
	}
	panic("unreachable")
}

func (tm *TypeMap) makeCommonType(t types.Type, k reflect.Kind) llvm.Value {
	// Not sure if there's an easier way to do this, but if you just
	// use ConstStruct, you end up getting a different llvm.Type.
	lt := tm.ToLLVM(t)
	typ := llvm.ConstNull(runtimeCommonType)
	elementTypes := runtimeCommonType.StructElementTypes()

	// Size.
	size := llvm.SizeOf(lt)
	if size.Type().IntTypeWidth() > elementTypes[0].IntTypeWidth() {
		size = llvm.ConstTrunc(size, elementTypes[0])
	}
	typ = llvm.ConstInsertValue(typ, size, []uint32{0})

	// TODO hash
	// TODO padding

	// Alignment.
	align := llvm.ConstTrunc(llvm.AlignOf(lt), llvm.Int8Type())
	typ = llvm.ConstInsertValue(typ, align, []uint32{3}) // var
	typ = llvm.ConstInsertValue(typ, align, []uint32{4}) // field

	// Kind.
	kind := llvm.ConstInt(llvm.Int8Type(), uint64(k), false)
	typ = llvm.ConstInsertValue(typ, kind, []uint32{5})

	// TODO alg
	// TODO string
	return typ
}

func (tm *TypeMap) badRuntimeType(b *types.Bad) llvm.Value {
	panic("bad type")
}

func (tm *TypeMap) basicRuntimeType(b *types.Basic) llvm.Value {
	commonType := tm.makeCommonType(b, reflect.Kind(b.Kind))
	result := llvm.AddGlobal(tm.module, commonType.Type(), "")
	ptr := llvm.ConstBitCast(result, runtimeCommonType.StructElementTypes()[9])
	commonType = llvm.ConstInsertValue(commonType, ptr, []uint32{9})
	result.SetInitializer(commonType)
	return result
}

func (tm *TypeMap) arrayRuntimeType(a *types.Array) llvm.Value {
	panic("unimplemented")
}

func (tm *TypeMap) sliceRuntimeType(s *types.Slice) llvm.Value {
	panic("unimplemented")
}

func (tm *TypeMap) structRuntimeType(s *types.Struct) llvm.Value {
	result := llvm.AddGlobal(tm.module, runtimeStructType, "")
	ptr := llvm.ConstBitCast(result, runtimeCommonType.StructElementTypes()[9])
	commonType := tm.makeCommonType(s, reflect.Struct)
	commonType = llvm.ConstInsertValue(commonType, ptr, []uint32{9})

	init := llvm.ConstNull(runtimeStructType)
	init = llvm.ConstInsertValue(init, commonType, []uint32{0})
	result.SetInitializer(init)

	// TODO set fields

	//panic("unimplemented")
	return result
}

func (tm *TypeMap) pointerRuntimeType(p *types.Pointer) llvm.Value {
	panic("unimplemented")
}

func (tm *TypeMap) funcRuntimeType(f *types.Func) llvm.Value {
	panic("unimplemented")
}

func (tm *TypeMap) interfaceRuntimeType(i *types.Interface) llvm.Value {
	panic("unimplemented")
}

func (tm *TypeMap) mapRuntimeType(m *types.Map) llvm.Value {
	panic("unimplemented")
}

func (tm *TypeMap) chanRuntimeType(c *types.Chan) llvm.Value {
	panic("unimplemented")
}

func (tm *TypeMap) nameRuntimeType(n *types.Name) llvm.Value {
	underlyingRuntimeType := tm.ToRuntime(n.Underlying).Initializer()
	result := llvm.AddGlobal(tm.module, underlyingRuntimeType.Type(), "")
	result.SetName("__llgo.reflect." + n.Obj.Name)
	ptr := llvm.ConstBitCast(result, runtimeCommonType.StructElementTypes()[9])
	commonType := llvm.ConstInsertValue(underlyingRuntimeType, ptr, []uint32{9})

	uncommonTypeInit := llvm.ConstNull(runtimeUncommonType) // TODO
	uncommonType := llvm.AddGlobal(tm.module, uncommonTypeInit.Type(), "")
	uncommonType.SetInitializer(uncommonTypeInit)
	commonType = llvm.ConstInsertValue(commonType, uncommonType, []uint32{8})

	// TODO set string, uncommonType.
	result.SetInitializer(commonType)
	return result
}

// vim: set ft=go :
