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
	"./types"
	"go/ast"
	"reflect"
)

type LLVMTypeMap struct {
	module llvm.Module
	target llvm.TargetData
	types  map[string]llvm.Type // compile-time LLVM type
}

type TypeMap struct {
	*LLVMTypeMap
	pkgpath   string
	types     map[string]llvm.Value // runtime/reflect type representation
	expr      map[ast.Expr]types.Type
	functions *FunctionCache
	resolver  Resolver

	// The runtime type of commonType.
	commonType               *types.Name
	commonTypePtrRuntimeType llvm.Value

	runtimeType,
	runtimeCommonType,
	runtimeUncommonType,
	runtimeArrayType,
	runtimeChanType,
	runtimeFuncType,
	runtimeMethod,
	runtimeImethod,
	runtimeInterfaceType,
	runtimeMapType,
	runtimePtrType,
	runtimeSliceType,
	runtimeStructType llvm.Type

	hashAlgFunctionType,
	equalAlgFunctionType,
	printAlgFunctionType,
	copyAlgFunctionType llvm.Type
}

func NewLLVMTypeMap(module llvm.Module, target llvm.TargetData) *LLVMTypeMap {
	tm := &LLVMTypeMap{module: module, target: target}
	tm.types = make(map[string]llvm.Type)
	return tm
}

func NewTypeMap(llvmtm *LLVMTypeMap, pkgpath string, exprTypes map[ast.Expr]types.Type, c *FunctionCache, r Resolver) *TypeMap {
	tm := &TypeMap{
		LLVMTypeMap: llvmtm,
		pkgpath:     pkgpath,
		types:       make(map[string]llvm.Value),
		expr:        exprTypes,
		functions:   c,
		resolver:    r,
	}

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
	tm.runtimeType = objToLLVMType("runtimeType")
	tm.runtimeCommonType = objToLLVMType("commonType")
	tm.runtimeUncommonType = objToLLVMType("uncommonType")
	tm.runtimeArrayType = objToLLVMType("arrayType")
	tm.runtimeChanType = objToLLVMType("chanType")
	tm.runtimeFuncType = objToLLVMType("funcType")
	tm.runtimeMethod = objToLLVMType("method")
	tm.runtimeImethod = objToLLVMType("imethod")
	tm.runtimeInterfaceType = objToLLVMType("interfaceType")
	tm.runtimeMapType = objToLLVMType("mapType")
	tm.runtimePtrType = objToLLVMType("ptrType")
	tm.runtimeSliceType = objToLLVMType("sliceType")
	tm.runtimeStructType = objToLLVMType("structType")
	tm.commonType = pkg.Scope.Lookup("commonType").Type.(*types.Name)

	// Types for algorithms. See 'runtime/runtime.h'.
	uintptrType := tm.target.IntPtrType()
	voidPtrType := llvm.PointerType(llvm.Int8Type(), 0)
	boolType := llvm.Int1Type()

	// Create runtime algorithm function types.
	params := []llvm.Type{uintptrType, voidPtrType}
	tm.hashAlgFunctionType = llvm.FunctionType(uintptrType, params, false)
	params = []llvm.Type{uintptrType, uintptrType, uintptrType}
	tm.equalAlgFunctionType = llvm.FunctionType(boolType, params, false)
	params = []llvm.Type{uintptrType, voidPtrType}
	tm.printAlgFunctionType = llvm.FunctionType(llvm.VoidType(), params, false)
	params = []llvm.Type{uintptrType, voidPtrType, voidPtrType}
	tm.copyAlgFunctionType = llvm.FunctionType(llvm.VoidType(), params, false)

	return tm
}

func (tm *LLVMTypeMap) ToLLVM(t types.Type) llvm.Type {
	t = types.Underlying(t)
	tstr := t.String()
	lt, ok := tm.types[tstr]
	if !ok {
		lt = tm.makeLLVMType(tstr, t)
		if lt.IsNil() {
			panic(fmt.Sprint("Failed to create LLVM type for: ", t))
		}
	}
	return lt
}

func (tm *TypeMap) ToRuntime(t types.Type) llvm.Value {
	tstr := t.String()
	r, ok := tm.types[tstr]
	if !ok {
		_, r = tm.makeRuntimeType(t)
		if r.IsNil() {
			panic(fmt.Sprint("Failed to create runtime type for: ", t))
		}
		tm.types[tstr] = r
	}
	return r
}

func (tm *LLVMTypeMap) makeLLVMType(tstr string, t types.Type) llvm.Type {
	switch t := t.(type) {
	case *types.Bad:
		return tm.badLLVMType(t)
	case *types.Basic:
		lt := tm.basicLLVMType(t)
		tm.types[tstr] = lt
		return lt
	case *types.Array:
		lt := tm.arrayLLVMType(t)
		tm.types[tstr] = lt
		return lt
	case *types.Slice:
		return tm.sliceLLVMType(tstr, t)
	case *types.Struct:
		return tm.structLLVMType(tstr, t)
	case *types.Pointer:
		lt := tm.pointerLLVMType(t)
		tm.types[tstr] = lt
		return lt
	case *types.Func:
		lt := tm.funcLLVMType(t)
		tm.types[tstr] = lt
		return lt
	case *types.Interface:
		return tm.interfaceLLVMType(tstr, t)
	case *types.Map:
		lt := tm.mapLLVMType(t)
		tm.types[tstr] = lt
		return lt
	case *types.Chan:
		lt := tm.chanLLVMType(t)
		tm.types[tstr] = lt
		return lt
	case *types.Name:
		lt := tm.nameLLVMType(t)
		tm.types[tstr] = lt
		return lt
	}
	panic("unreachable")
}

func (tm *LLVMTypeMap) badLLVMType(b *types.Bad) llvm.Type {
	return llvm.Type{nil}
}

func (tm *LLVMTypeMap) basicLLVMType(b *types.Basic) llvm.Type {
	switch b.Kind {
	case types.BoolKind:
		return llvm.Int1Type()
	case types.Int8Kind, types.Uint8Kind:
		return llvm.Int8Type()
	case types.Int16Kind, types.Uint16Kind:
		return llvm.Int16Type()
	case types.Int32Kind, types.Uint32Kind, types.UintKind, types.IntKind:
		return llvm.Int32Type()
	case types.Int64Kind, types.Uint64Kind:
		return llvm.Int64Type()
	case types.Float32Kind:
		return llvm.FloatType()
	case types.Float64Kind:
		return llvm.DoubleType()
	case types.UnsafePointerKind, types.UintptrKind:
		return tm.target.IntPtrType()
	case types.Complex64Kind:
		f32 := llvm.FloatType()
		elements := []llvm.Type{f32, f32}
		return llvm.StructType(elements, false)
	case types.Complex128Kind:
		f64 := llvm.DoubleType()
		elements := []llvm.Type{f64, f64}
		return llvm.StructType(elements, false)
	//case UntypedInt:
	//case UntypedFloat:
	//case UntypedComplex:
	case types.StringKind:
		i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
		elements := []llvm.Type{i8ptr, llvm.Int32Type()}
		return llvm.StructType(elements, false)
	}
	panic(fmt.Sprint("unhandled kind: ", b.Kind))
}

func (tm *LLVMTypeMap) arrayLLVMType(a *types.Array) llvm.Type {
	return llvm.ArrayType(tm.ToLLVM(a.Elt), int(a.Len))
}

func (tm *LLVMTypeMap) sliceLLVMType(tstr string, s *types.Slice) llvm.Type {
	typ, ok := tm.types[tstr]
	if !ok {
		typ = llvm.GlobalContext().StructCreateNamed("")
		tm.types[tstr] = typ
		elements := []llvm.Type{
			llvm.PointerType(tm.ToLLVM(s.Elt), 0),
			tm.ToLLVM(types.Uint),
			tm.ToLLVM(types.Uint),
		}
		typ.StructSetBody(elements, false)
	}
	return typ
}

func (tm *LLVMTypeMap) structLLVMType(tstr string, s *types.Struct) llvm.Type {
	typ, ok := tm.types[tstr]
	if !ok {
		typ = llvm.GlobalContext().StructCreateNamed("")
		tm.types[tstr] = typ
		elements := make([]llvm.Type, len(s.Fields))
		for i, f := range s.Fields {
			ft := f.Type.(types.Type)
			elements[i] = tm.ToLLVM(ft)
		}
		typ.StructSetBody(elements, false)
	}
	return typ
}

func (tm *LLVMTypeMap) pointerLLVMType(p *types.Pointer) llvm.Type {
	return llvm.PointerType(tm.ToLLVM(p.Base), 0)
}

func (tm *LLVMTypeMap) funcLLVMType(f *types.Func) llvm.Type {
	param_types := make([]llvm.Type, 0)

	// Add receiver parameter.
	if f.Recv != nil {
		recv_type := f.Recv.Type.(types.Type)
		param_types = append(param_types, tm.ToLLVM(recv_type))
	}

	for _, param := range f.Params {
		param_type := param.Type.(types.Type)
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

func (tm *LLVMTypeMap) interfaceLLVMType(tstr string, i *types.Interface) llvm.Type {
	typ, ok := tm.types[tstr]
	if !ok {
		typ = llvm.GlobalContext().StructCreateNamed("")
		tm.types[tstr] = typ
		valptr_type := llvm.PointerType(llvm.Int8Type(), 0)
		typptr_type := valptr_type // runtimeCommonType may not be defined yet
		elements := make([]llvm.Type, 2+len(i.Methods))
		elements[0] = typptr_type // type
		elements[1] = valptr_type // value
		for n, m := range i.Methods {
			// Add an opaque pointer parameter to the function for the
			// struct pointer.
			fntype := m.Type.(*types.Func)
			receiver_type := &types.Pointer{Base: types.Int8}
			fntype.Recv = ast.NewObj(ast.Var, "")
			fntype.Recv.Type = receiver_type
			elements[n+2] = tm.ToLLVM(fntype)
			fntype.Recv = nil
		}
		typ.StructSetBody(elements, false)
	}
	return typ
}

func (tm *LLVMTypeMap) mapLLVMType(m *types.Map) llvm.Type {
	// All map details are in the runtime. We represent it here as an
	// opaque pointer.
	return tm.target.IntPtrType()
}

func (tm *LLVMTypeMap) chanLLVMType(c *types.Chan) llvm.Type {
	// All channel details are in the runtime. We represent it
	// here as an opaque pointer.
	return tm.target.IntPtrType()
}

func (tm *LLVMTypeMap) nameLLVMType(n *types.Name) llvm.Type {
	return tm.ToLLVM(n.Underlying)
}

///////////////////////////////////////////////////////////////////////////////

func (tm *TypeMap) makeRuntimeType(t types.Type) (global, ptr llvm.Value) {
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

func (tm *TypeMap) makeAlgorithmTable(t types.Type) llvm.Value {
	// TODO set these to actual functions.
	hashAlg := llvm.ConstNull(llvm.PointerType(tm.hashAlgFunctionType, 0))
	printAlg := llvm.ConstNull(llvm.PointerType(tm.printAlgFunctionType, 0))
	copyAlg := llvm.ConstNull(llvm.PointerType(tm.copyAlgFunctionType, 0))

	equalAlg := tm.functions.NamedFunction("runtime.memequal", "func f(uintptr, unsafe.Pointer, unsafe.Pointer) bool")
	elems := []llvm.Value{hashAlg, equalAlg, printAlg, copyAlg}
	return llvm.ConstStruct(elems, false)
}

func (tm *TypeMap) makeRuntimeTypeGlobal(v llvm.Value) (global, ptr llvm.Value) {
	initType := llvm.StructType([]llvm.Type{tm.runtimeType, v.Type()}, false)
	global = llvm.AddGlobal(tm.module, initType, "")
	ptr = llvm.ConstBitCast(global, llvm.PointerType(tm.runtimeType, 0))

	// Set ptrToThis in v's commonType.
	if v.Type() == tm.runtimeCommonType {
		v = llvm.ConstInsertValue(v, ptr, []uint32{10})
	} else {
		commonType := llvm.ConstExtractValue(v, []uint32{0})
		commonType = llvm.ConstInsertValue(commonType, ptr, []uint32{10})
		v = llvm.ConstInsertValue(v, commonType, []uint32{0})
	}

	// interface{} containing v's *commonType representation.
	runtimeTypeValue := llvm.Undef(tm.runtimeType)
	zero := llvm.ConstNull(llvm.Int32Type())
	one := llvm.ConstInt(llvm.Int32Type(), 1, false)
	i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
	if tm.commonTypePtrRuntimeType.IsNil() {
		// Create a dummy pointer value, which we'll update straight after
		// defining the runtime type info for commonType.
		tm.commonTypePtrRuntimeType = llvm.Undef(i8ptr)
		commonTypePtr := &types.Pointer{Base: tm.commonType}
		commonTypeGlobal, commonTypeRuntimeType := tm.makeRuntimeType(tm.commonType)
		tm.types[tm.commonType.String()] = commonTypeRuntimeType
		commonTypePtrGlobal, commonTypePtrRuntimeType := tm.makeRuntimeType(commonTypePtr)
		tm.types[commonTypePtr.String()] = commonTypePtrRuntimeType
		tm.commonTypePtrRuntimeType = llvm.ConstBitCast(commonTypePtrRuntimeType, i8ptr)
		if tm.pkgpath == tm.commonType.Package {
			// Update the interace{} header of the commonType/*commonType
			// runtime types we just created.
			for _, g := range [...]llvm.Value{commonTypeGlobal, commonTypePtrGlobal} {
				init := g.Initializer()
				typptr := tm.commonTypePtrRuntimeType
				runtimeTypeValue := llvm.ConstExtractValue(init, []uint32{0})
				runtimeTypeValue = llvm.ConstInsertValue(runtimeTypeValue, typptr, []uint32{0})
				init = llvm.ConstInsertValue(init, runtimeTypeValue, []uint32{0})
				g.SetInitializer(init)
			}
		}

	}
	commonTypePtr := llvm.ConstGEP(global, []llvm.Value{zero, one})
	commonTypePtr = llvm.ConstBitCast(commonTypePtr, i8ptr)
	runtimeTypeValue = llvm.ConstInsertValue(runtimeTypeValue, tm.commonTypePtrRuntimeType, []uint32{0})
	runtimeTypeValue = llvm.ConstInsertValue(runtimeTypeValue, commonTypePtr, []uint32{1})

	init := llvm.Undef(initType)
	init = llvm.ConstInsertValue(init, runtimeTypeValue, []uint32{0})
	init = llvm.ConstInsertValue(init, v, []uint32{1})
	global.SetInitializer(init)

	return
}

func (tm *TypeMap) makeCommonType(t types.Type, k reflect.Kind) llvm.Value {
	// Not sure if there's an easier way to do this, but if you just
	// use ConstStruct, you end up getting a different llvm.Type.
	lt := tm.ToLLVM(t)
	typ := llvm.ConstNull(tm.runtimeCommonType)
	elementTypes := tm.runtimeCommonType.StructElementTypes()

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

	// Algorithm table.
	alg := tm.makeAlgorithmTable(t)
	algptr := llvm.AddGlobal(tm.module, alg.Type(), "")
	algptr.SetInitializer(alg)
	algptr = llvm.ConstBitCast(algptr, elementTypes[6])
	typ = llvm.ConstInsertValue(typ, algptr, []uint32{6})

	// String representation.
	stringrep := tm.globalStringPtr(t.String())
	typ = llvm.ConstInsertValue(typ, stringrep, []uint32{8})

	// TODO gc
	return typ
}

func (tm *TypeMap) badRuntimeType(b *types.Bad) (global, ptr llvm.Value) {
	panic("bad type")
}

func (tm *TypeMap) basicRuntimeType(b *types.Basic) (global, ptr llvm.Value) {
	commonType := tm.makeCommonType(b, reflect.Kind(b.Kind))
	return tm.makeRuntimeTypeGlobal(commonType)
}

func (tm *TypeMap) arrayRuntimeType(a *types.Array) (global, ptr llvm.Value) {
	panic("unimplemented")
}

func (tm *TypeMap) sliceRuntimeType(s *types.Slice) (global, ptr llvm.Value) {
	commonType := tm.makeCommonType(s, reflect.Slice)
	elemRuntimeType := tm.ToRuntime(s.Elt)
	sliceType := llvm.ConstNull(tm.runtimeSliceType)
	sliceType = llvm.ConstInsertValue(sliceType, commonType, []uint32{0})
	sliceType = llvm.ConstInsertValue(sliceType, elemRuntimeType, []uint32{1})
	return tm.makeRuntimeTypeGlobal(sliceType)
}

func (tm *TypeMap) structRuntimeType(s *types.Struct) (global, ptr llvm.Value) {
	commonType := tm.makeCommonType(s, reflect.Struct)
	structType := llvm.ConstNull(tm.runtimeStructType)
	structType = llvm.ConstInsertValue(structType, commonType, []uint32{0})
	// TODO set fields
	return tm.makeRuntimeTypeGlobal(structType)
}

func (tm *TypeMap) pointerRuntimeType(p *types.Pointer) (global, ptr llvm.Value) {
	commonType := tm.makeCommonType(p, reflect.Ptr)
	ptrType := llvm.ConstNull(tm.runtimePtrType)
	ptrType = llvm.ConstInsertValue(ptrType, commonType, []uint32{0})
	ptrType = llvm.ConstInsertValue(ptrType, tm.ToRuntime(p.Base), []uint32{1})
	return tm.makeRuntimeTypeGlobal(ptrType)
}

func (tm *TypeMap) funcRuntimeType(f *types.Func) (global, ptr llvm.Value) {
	commonType := tm.makeCommonType(f, reflect.Func)
	funcType := llvm.ConstNull(tm.runtimeFuncType)
	funcType = llvm.ConstInsertValue(funcType, commonType, []uint32{0})
	// dotdotdot
	if f.IsVariadic {
		variadic := llvm.ConstInt(llvm.Int1Type(), 1, false)
		funcType = llvm.ConstInsertValue(funcType, variadic, []uint32{1})
	}
	// TODO in
	//funcType = llvm.ConstInsertValue(funcType, tm.ToRuntime(p.Base), []uint32{2})
	// TODO out
	//funcType = llvm.ConstInsertValue(funcType, tm.ToRuntime(p.Base), []uint32{3})
	return tm.makeRuntimeTypeGlobal(funcType)
}

func (tm *TypeMap) interfaceRuntimeType(i *types.Interface) (global, ptr llvm.Value) {
	commonType := tm.makeCommonType(i, reflect.Interface)
	interfaceType := llvm.ConstNull(tm.runtimeInterfaceType)
	interfaceType = llvm.ConstInsertValue(interfaceType, commonType, []uint32{0})

	imethods := make([]llvm.Value, len(i.Methods))
	for index, method := range i.Methods {
		//name, pkgPath, type
		imethod := llvm.ConstNull(tm.runtimeImethod)
		name := tm.globalStringPtr(method.Name)
		name = llvm.ConstBitCast(name, tm.runtimeImethod.StructElementTypes()[0])

		imethod = llvm.ConstInsertValue(imethod, name, []uint32{0})
		//imethod = llvm.ConstInsertValue(imethod, , []uint32{1})
		//imethod = llvm.ConstInsertValue(imethod, , []uint32{2})
		imethods[index] = imethod
	}

	var imethodsGlobalPtr llvm.Value
	imethodPtrType := llvm.PointerType(tm.runtimeImethod, 0)
	if len(imethods) > 0 {
		imethodsArray := llvm.ConstArray(tm.runtimeImethod, imethods)
		imethodsGlobalPtr = llvm.AddGlobal(tm.module, imethodsArray.Type(), "")
		imethodsGlobalPtr.SetInitializer(imethodsArray)
		imethodsGlobalPtr = llvm.ConstBitCast(imethodsGlobalPtr, imethodPtrType)
	} else {
		imethodsGlobalPtr = llvm.ConstNull(imethodPtrType)
	}

	len_ := llvm.ConstInt(llvm.Int32Type(), uint64(len(i.Methods)), false)
	imethodsSliceType := tm.runtimeInterfaceType.StructElementTypes()[1]
	imethodsSlice := llvm.ConstNull(imethodsSliceType)
	imethodsSlice = llvm.ConstInsertValue(imethodsSlice, imethodsGlobalPtr, []uint32{0})
	imethodsSlice = llvm.ConstInsertValue(imethodsSlice, len_, []uint32{1})
	imethodsSlice = llvm.ConstInsertValue(imethodsSlice, len_, []uint32{2})
	interfaceType = llvm.ConstInsertValue(interfaceType, imethodsSlice, []uint32{1})
	return tm.makeRuntimeTypeGlobal(interfaceType)
}

func (tm *TypeMap) mapRuntimeType(m *types.Map) (global, ptr llvm.Value) {
	commonType := tm.makeCommonType(m, reflect.Map)
	mapType := llvm.ConstNull(tm.runtimeMapType)
	mapType = llvm.ConstInsertValue(mapType, commonType, []uint32{0})
	mapType = llvm.ConstInsertValue(mapType, tm.ToRuntime(m.Key), []uint32{1})
	mapType = llvm.ConstInsertValue(mapType, tm.ToRuntime(m.Elt), []uint32{2})
	return tm.makeRuntimeTypeGlobal(mapType)
}

func (tm *TypeMap) chanRuntimeType(c *types.Chan) (global, ptr llvm.Value) {
	commonType := tm.makeCommonType(c, reflect.Chan)
	chanType := llvm.ConstNull(tm.runtimeChanType)
	chanType = llvm.ConstInsertValue(chanType, commonType, []uint32{0})
	chanType = llvm.ConstInsertValue(chanType, tm.ToRuntime(c.Elt), []uint32{1})

	// go/ast and reflect disagree on values for direction.
	var dir reflect.ChanDir
	if c.Dir&ast.SEND != 0 {
		dir = reflect.SendDir
	}
	if c.Dir&ast.RECV != 0 {
		dir |= reflect.RecvDir
	}
	uintptrdir := llvm.ConstInt(tm.target.IntPtrType(), uint64(dir), false)
	chanType = llvm.ConstInsertValue(chanType, uintptrdir, []uint32{2})
	return tm.makeRuntimeTypeGlobal(chanType)
}

func (tm *TypeMap) nameRuntimeType(n *types.Name) (global, ptr llvm.Value) {
	builtin := false
	pkgpath := n.Package
	if pkgpath == "" {
		// Set to "runtime", so the builtin types have a home.
		pkgpath = "runtime"
		builtin = true
	}
	globalname := "__llgo.type.name." + n.String()
	if pkgpath != tm.pkgpath {
		// We're not compiling the package from whence the type came,
		// so we'll just create a pointer to it here.
		global := llvm.AddGlobal(tm.module, tm.runtimeType, globalname)
		global.SetInitializer(llvm.ConstNull(tm.runtimeType))
		global.SetLinkage(llvm.CommonLinkage)
		return global, global
	}

	underlying := n.Underlying
	if name, ok := underlying.(*types.Name); ok {
		underlying = name.Underlying
	}

	global, ptr = tm.makeRuntimeType(underlying)
	globalInit := global.Initializer()

	// Locate the common type.
	underlyingRuntimeType := llvm.ConstExtractValue(globalInit, []uint32{1})
	commonType := underlyingRuntimeType
	if underlyingRuntimeType.Type() != tm.runtimeCommonType {
		commonType = llvm.ConstExtractValue(commonType, []uint32{0})
	}

	// Insert the uncommon type.
	uncommonTypeInit := llvm.ConstNull(tm.runtimeUncommonType)
	namePtr := tm.globalStringPtr(n.Obj.Name)
	uncommonTypeInit = llvm.ConstInsertValue(uncommonTypeInit, namePtr, []uint32{0})
	var pkgpathPtr llvm.Value
	if !builtin {
		pkgpathPtr = tm.globalStringPtr(pkgpath)
		uncommonTypeInit = llvm.ConstInsertValue(uncommonTypeInit, pkgpathPtr, []uint32{1})
	}

	// Replace the commonType's string representation.
	commonType = llvm.ConstInsertValue(commonType, namePtr, []uint32{8})

	methods := make([]llvm.Value, len(n.Methods))
	for index, m := range n.Methods {
		method := llvm.ConstNull(tm.runtimeMethod)
		name := tm.globalStringPtr(m.Name)
		name = llvm.ConstBitCast(name, tm.runtimeMethod.StructElementTypes()[0])
		// name
		method = llvm.ConstInsertValue(method, name, []uint32{0})
		// pkgPath
		method = llvm.ConstInsertValue(method, pkgpathPtr, []uint32{1})
		// mtyp (method type, no receiver)
		ftyp := m.Type.(*types.Func)
		{
			recv := ftyp.Recv
			ftyp.Recv = nil
			mtyp := tm.ToRuntime(ftyp)
			method = llvm.ConstInsertValue(method, mtyp, []uint32{2})
			ftyp.Recv = recv
		}
		// typ (function type, with receiver)
		typ := tm.ToRuntime(ftyp)
		method = llvm.ConstInsertValue(method, typ, []uint32{3})
		// ifn (single-word receiver function pointer for interface calls)
		ifn := tm.resolver.Resolve(m).LLVMValue() // TODO generate trampoline as necessary.
		ifn = llvm.ConstPtrToInt(ifn, tm.target.IntPtrType())
		method = llvm.ConstInsertValue(method, ifn, []uint32{4})
		// tfn (standard method/function pointer for plain method calls)
		tfn := tm.resolver.Resolve(m).LLVMValue()
		tfn = llvm.ConstPtrToInt(tfn, tm.target.IntPtrType())
		method = llvm.ConstInsertValue(method, tfn, []uint32{5})
		methods[index] = method
	}

	var methodsGlobalPtr llvm.Value
	if len(methods) > 0 {
		methodsArray := llvm.ConstArray(tm.runtimeMethod, methods)
		methodsGlobalPtr = llvm.AddGlobal(tm.module, methodsArray.Type(), "")
		methodsGlobalPtr.SetInitializer(methodsArray)
		i32zero := llvm.ConstNull(llvm.Int32Type())
		methodsGlobalPtr = llvm.ConstGEP(methodsGlobalPtr, []llvm.Value{i32zero, i32zero})
	} else {
		methodsGlobalPtr = llvm.ConstNull(llvm.PointerType(tm.runtimeMethod, 0))
	}
	len_ := llvm.ConstInt(llvm.Int32Type(), uint64(len(methods)), false)
	methodsSliceType := tm.runtimeUncommonType.StructElementTypes()[2]
	methodsSlice := llvm.ConstNull(methodsSliceType)
	methodsSlice = llvm.ConstInsertValue(methodsSlice, methodsGlobalPtr, []uint32{0})
	methodsSlice = llvm.ConstInsertValue(methodsSlice, len_, []uint32{1})
	methodsSlice = llvm.ConstInsertValue(methodsSlice, len_, []uint32{2})
	uncommonTypeInit = llvm.ConstInsertValue(uncommonTypeInit, methodsSlice, []uint32{2})

	uncommonType := llvm.AddGlobal(tm.module, uncommonTypeInit.Type(), "")
	uncommonType.SetInitializer(uncommonTypeInit)
	commonType = llvm.ConstInsertValue(commonType, uncommonType, []uint32{9})

	// Update the global's initialiser. Note that we take a copy
	// of the underlying type; we're not updating a shared type.
	if underlyingRuntimeType.Type() != tm.runtimeCommonType {
		underlyingRuntimeType = llvm.ConstInsertValue(underlyingRuntimeType, commonType, []uint32{0})
	} else {
		underlyingRuntimeType = commonType
	}
	globalInit = llvm.ConstInsertValue(globalInit, underlyingRuntimeType, []uint32{1})
	global.SetName(globalname)
	global.SetInitializer(globalInit)
	return global, ptr
}

// globalStringPtr returns a *string with the specified value.
func (tm *TypeMap) globalStringPtr(value string) llvm.Value {
	strval := llvm.ConstString(value, false)
	strglobal := llvm.AddGlobal(tm.module, strval.Type(), "")
	strglobal.SetInitializer(strval)
	strglobal = llvm.ConstBitCast(strglobal, llvm.PointerType(llvm.Int8Type(), 0))
	strlen := llvm.ConstInt(llvm.Int32Type(), uint64(len(value)), false)
	str := llvm.ConstStruct([]llvm.Value{strglobal, strlen}, false)
	g := llvm.AddGlobal(tm.module, str.Type(), "")
	g.SetInitializer(str)
	return g
}

// vim: set ft=go :
