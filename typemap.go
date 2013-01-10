// Copyright 2011 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"fmt"
	"github.com/axw/gollvm/llvm"
	"go/ast"
	"go/types"
	"reflect"
	"sort"
)

type ExprTypeInfo struct {
	types.Type

	// Constant value if non-nil.
	Value interface{}
}

type ExprTypeMap map[ast.Expr]ExprTypeInfo

type LLVMTypeMap struct {
	*TypeStringer
	target  llvm.TargetData
	inttype llvm.Type
	types   map[string]llvm.Type // compile-time LLVM type
}

type runtimeTypeInfo struct {
	global llvm.Value
	dyntyp llvm.Value
}

type TypeMap struct {
	*LLVMTypeMap
	module    llvm.Module
	pkgpath   string
	types     map[string]runtimeTypeInfo
	expr      ExprTypeMap
	functions *FunctionCache
	resolver  Resolver

	runtimeType,
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

func NewLLVMTypeMap(target llvm.TargetData, ts *TypeStringer) *LLVMTypeMap {
	// spec says int is either 32-bit or 64-bit.
	var inttype llvm.Type
	if target.PointerSize() >= 8 {
		inttype = llvm.Int64Type()
	} else {
		inttype = llvm.Int32Type()
	}
	return &LLVMTypeMap{
		TypeStringer: ts,
		target:       target,
		types:        make(map[string]llvm.Type),
		inttype:      inttype,
	}
}

func NewTypeMap(llvmtm *LLVMTypeMap, module llvm.Module, pkgpath string, exprTypes ExprTypeMap, c *FunctionCache, r Resolver) *TypeMap {
	tm := &TypeMap{
		LLVMTypeMap: llvmtm,
		module:      module,
		pkgpath:     pkgpath,
		types:       make(map[string]runtimeTypeInfo),
		expr:        exprTypes,
		functions:   c,
		resolver:    r,
	}

	// Load runtime/reflect types, and generate LLVM types for
	// the structures we need to populate runtime type information.
	pkg, err := c.compiler.parseReflect()
	if err != nil {
		panic(err) // FIXME return err
	}
	reflectLLVMType := func(name string) llvm.Type {
		obj := pkg.Scope.Lookup(name)
		if obj == nil {
			panic(fmt.Errorf("Failed to find type: %s", name))
		}
		return tm.ToLLVM(obj.Type.(types.Type))
	}
	tm.runtimeType = reflectLLVMType("rtype")
	tm.runtimeUncommonType = reflectLLVMType("uncommonType")
	tm.runtimeArrayType = reflectLLVMType("arrayType")
	tm.runtimeChanType = reflectLLVMType("chanType")
	tm.runtimeFuncType = reflectLLVMType("funcType")
	tm.runtimeMethod = reflectLLVMType("method")
	tm.runtimeImethod = reflectLLVMType("imethod")
	tm.runtimeInterfaceType = reflectLLVMType("interfaceType")
	tm.runtimeMapType = reflectLLVMType("mapType")
	tm.runtimePtrType = reflectLLVMType("ptrType")
	tm.runtimeSliceType = reflectLLVMType("sliceType")
	tm.runtimeStructType = reflectLLVMType("structType")

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
	tstr := tm.TypeString(t)
	lt, ok := tm.types[tstr]
	if !ok {
		lt = tm.makeLLVMType(tstr, t)
		if lt.IsNil() {
			panic(fmt.Sprint("Failed to create LLVM type for: ", tstr))
		}
	}
	return lt
}

func (tm *TypeMap) ToRuntime(t types.Type) llvm.Value {
	_, r := tm.toRuntime(t)
	return r
}

func (tm *TypeMap) toRuntime(t types.Type) (global, value llvm.Value) {
	tstr := tm.TypeString(t)
	info, ok := tm.types[tstr]
	if !ok {
		info.global, info.dyntyp = tm.makeRuntimeType(t)
		if info.dyntyp.IsNil() {
			panic(fmt.Sprint("Failed to create runtime type for: ", tstr))
		}
		tm.types[tstr] = info
	}
	return info.global, info.dyntyp
}

func (tm *LLVMTypeMap) makeLLVMType(tstr string, t types.Type) llvm.Type {
	switch t := t.(type) {
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
	case *types.Signature:
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
	case *types.NamedType:
		lt := tm.nameLLVMType(t)
		tm.types[tstr] = lt
		return lt
	}
	panic(fmt.Errorf("unhandled: %T", t))
}

func (tm *LLVMTypeMap) basicLLVMType(b *types.Basic) llvm.Type {
	switch b.Kind {
	case types.Bool:
		return llvm.Int1Type()
	case types.Int8, types.Uint8:
		return llvm.Int8Type()
	case types.Int16, types.Uint16:
		return llvm.Int16Type()
	case types.Int32, types.Uint32:
		return llvm.Int32Type()
	case types.Uint, types.Int:
		return tm.inttype
	case types.Int64, types.Uint64:
		return llvm.Int64Type()
	case types.Float32:
		return llvm.FloatType()
	case types.Float64:
		return llvm.DoubleType()
	case types.UnsafePointer, types.Uintptr:
		return tm.target.IntPtrType()
	case types.Complex64:
		f32 := llvm.FloatType()
		elements := []llvm.Type{f32, f32}
		return llvm.StructType(elements, false)
	case types.Complex128:
		f64 := llvm.DoubleType()
		elements := []llvm.Type{f64, f64}
		return llvm.StructType(elements, false)
	case types.String:
		i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
		elements := []llvm.Type{i8ptr, tm.inttype}
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
			tm.inttype,
			tm.inttype,
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

func (tm *LLVMTypeMap) funcLLVMType(f *types.Signature) llvm.Type {
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

func (tm *LLVMTypeMap) interfaceLLVMType(tstr string, i *types.Interface) llvm.Type {
	typ, ok := tm.types[tstr]
	if !ok {
		typ = llvm.GlobalContext().StructCreateNamed("")
		tm.types[tstr] = typ
		valptr_type := llvm.PointerType(llvm.Int8Type(), 0)
		typptr_type := valptr_type // runtimeType may not be defined yet
		elements := make([]llvm.Type, 2+len(i.Methods))
		elements[0] = typptr_type // type
		elements[1] = valptr_type // value
		for n, m := range i.Methods {
			// Add an opaque pointer parameter to the function for the
			// struct pointer. Take a copy of the Type here, so we don't
			// change how the Interface's TypeString is determined.
			fntype := *m.Type
			recvtyp := &types.Pointer{Base: types.Typ[types.Int8]}
			fntype.Recv = &types.Var{Type: recvtyp}
			elements[n+2] = tm.ToLLVM(&fntype)
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

func (tm *LLVMTypeMap) nameLLVMType(n *types.NamedType) llvm.Type {
	return tm.ToLLVM(n.Underlying)
}

func (tm *LLVMTypeMap) Sizeof(t types.Type) uint64 {
	return tm.target.TypeAllocSize(tm.ToLLVM(t))
}

///////////////////////////////////////////////////////////////////////////////

func (tm *TypeMap) makeRuntimeType(t types.Type) (global, ptr llvm.Value) {
	switch t := t.(type) {
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
	case *types.Signature:
		return tm.funcRuntimeType(t)
	case *types.Interface:
		return tm.interfaceRuntimeType(t)
	case *types.Map:
		return tm.mapRuntimeType(t)
	case *types.Chan:
		return tm.chanRuntimeType(t)
	case *types.NamedType:
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
	global = llvm.AddGlobal(tm.module, v.Type(), "")
	global.SetInitializer(v)
	ptr = llvm.ConstBitCast(global, llvm.PointerType(tm.runtimeType, 0))
	return global, ptr
}

func (tm *TypeMap) makeRtype(t types.Type, k reflect.Kind) llvm.Value {
	// Not sure if there's an easier way to do this, but if you just
	// use ConstStruct, you end up getting a different llvm.Type.
	lt := tm.ToLLVM(t)
	typ := llvm.ConstNull(tm.runtimeType)
	elementTypes := tm.runtimeType.StructElementTypes()

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
	stringrep := tm.globalStringPtr(tm.TypeString(t))
	typ = llvm.ConstInsertValue(typ, stringrep, []uint32{8})

	// TODO gc
	return typ
}

var basicReflectKinds = [...]reflect.Kind{
	types.Invalid:       reflect.Invalid,
	types.Bool:          reflect.Bool,
	types.Int:           reflect.Int,
	types.Int8:          reflect.Int8,
	types.Int16:         reflect.Int16,
	types.Int32:         reflect.Int32,
	types.Int64:         reflect.Int64,
	types.Uint:          reflect.Uint,
	types.Uint8:         reflect.Uint8,
	types.Uint16:        reflect.Uint16,
	types.Uint32:        reflect.Uint32,
	types.Uint64:        reflect.Uint64,
	types.Uintptr:       reflect.Uintptr,
	types.Float32:       reflect.Float32,
	types.Float64:       reflect.Float64,
	types.Complex64:     reflect.Complex64,
	types.Complex128:    reflect.Complex128,
	types.String:        reflect.String,
	types.UnsafePointer: reflect.UnsafePointer,
}

func (tm *TypeMap) basicRuntimeType(b *types.Basic) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(b, basicReflectKinds[b.Kind])
	return tm.makeRuntimeTypeGlobal(rtype)
}

func (tm *TypeMap) arrayRuntimeType(a *types.Array) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(a, reflect.Array)
	elemRuntimeType := tm.ToRuntime(a.Elt)
	sliceRuntimeType := tm.ToRuntime(&types.Slice{Elt: a.Elt})
	uintptrlen := llvm.ConstInt(tm.target.IntPtrType(), uint64(a.Len), false)
	arrayType := llvm.ConstNull(tm.runtimeArrayType)
	arrayType = llvm.ConstInsertValue(arrayType, rtype, []uint32{0})
	arrayType = llvm.ConstInsertValue(arrayType, elemRuntimeType, []uint32{1})
	arrayType = llvm.ConstInsertValue(arrayType, sliceRuntimeType, []uint32{2})
	arrayType = llvm.ConstInsertValue(arrayType, uintptrlen, []uint32{3})
	return tm.makeRuntimeTypeGlobal(arrayType)
}

func (tm *TypeMap) sliceRuntimeType(s *types.Slice) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(s, reflect.Slice)
	elemRuntimeType := tm.ToRuntime(s.Elt)
	sliceType := llvm.ConstNull(tm.runtimeSliceType)
	sliceType = llvm.ConstInsertValue(sliceType, rtype, []uint32{0})
	sliceType = llvm.ConstInsertValue(sliceType, elemRuntimeType, []uint32{1})
	return tm.makeRuntimeTypeGlobal(sliceType)
}

func (tm *TypeMap) structRuntimeType(s *types.Struct) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(s, reflect.Struct)
	structType := llvm.ConstNull(tm.runtimeStructType)
	structType = llvm.ConstInsertValue(structType, rtype, []uint32{0})
	// TODO set fields
	return tm.makeRuntimeTypeGlobal(structType)
}

func (tm *TypeMap) pointerRuntimeType(p *types.Pointer) (global, ptr llvm.Value) {
	// Is the base type a named type from another package? If so, we'll
	// create a reference to the externally defined symbol.
	var globalname string
	if n, ok := p.Base.(*types.NamedType); ok {
		// FIXME horrible circular relationship
		pkgpath := tm.functions.pkgmap[n.Obj]
		if pkgpath == "" {
			pkgpath = "runtime"
		}
		globalname = "__llgo.type.*" + pkgpath + "." + n.Obj.Name
		if pkgpath != tm.pkgpath {
			global := llvm.AddGlobal(tm.module, tm.runtimeType, globalname)
			global.SetInitializer(llvm.ConstNull(tm.runtimeType))
			global.SetLinkage(llvm.CommonLinkage)
			return global, global
		}
	}

	rtype := tm.makeRtype(p, reflect.Ptr)
	if n, ok := p.Base.(*types.NamedType); ok {
		uncommonTypeInit := tm.uncommonType(n, true)
		uncommonType := llvm.AddGlobal(tm.module, uncommonTypeInit.Type(), "")
		uncommonType.SetInitializer(uncommonTypeInit)
		rtype = llvm.ConstInsertValue(rtype, uncommonType, []uint32{9})
	}

	baseTypeGlobal, baseTypePtr := tm.toRuntime(p.Base)
	ptrType := llvm.ConstNull(tm.runtimePtrType)
	ptrType = llvm.ConstInsertValue(ptrType, rtype, []uint32{0})
	ptrType = llvm.ConstInsertValue(ptrType, baseTypePtr, []uint32{1})
	global, ptr = tm.makeRuntimeTypeGlobal(ptrType)
	global.SetName(globalname)

	// Set ptrToThis in the base type's rtype.
	baseType := baseTypeGlobal.Initializer()
	if baseType.Type() == tm.runtimeType {
		baseType = llvm.ConstInsertValue(baseType, ptr, []uint32{10})
	} else {
		rtype := llvm.ConstExtractValue(baseType, []uint32{0})
		rtype = llvm.ConstInsertValue(rtype, ptr, []uint32{10})
		baseType = llvm.ConstInsertValue(baseType, rtype, []uint32{0})
	}
	baseTypeGlobal.SetInitializer(baseType)

	return global, ptr
}

func (tm *TypeMap) funcRuntimeType(f *types.Signature) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(f, reflect.Func)
	funcType := llvm.ConstNull(tm.runtimeFuncType)
	funcType = llvm.ConstInsertValue(funcType, rtype, []uint32{0})
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
	rtype := tm.makeRtype(i, reflect.Interface)
	interfaceType := llvm.ConstNull(tm.runtimeInterfaceType)
	interfaceType = llvm.ConstInsertValue(interfaceType, rtype, []uint32{0})

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

	len_ := llvm.ConstInt(tm.inttype, uint64(len(i.Methods)), false)
	imethodsSliceType := tm.runtimeInterfaceType.StructElementTypes()[1]
	imethodsSlice := llvm.ConstNull(imethodsSliceType)
	imethodsSlice = llvm.ConstInsertValue(imethodsSlice, imethodsGlobalPtr, []uint32{0})
	imethodsSlice = llvm.ConstInsertValue(imethodsSlice, len_, []uint32{1})
	imethodsSlice = llvm.ConstInsertValue(imethodsSlice, len_, []uint32{2})
	interfaceType = llvm.ConstInsertValue(interfaceType, imethodsSlice, []uint32{1})
	return tm.makeRuntimeTypeGlobal(interfaceType)
}

func (tm *TypeMap) mapRuntimeType(m *types.Map) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(m, reflect.Map)
	mapType := llvm.ConstNull(tm.runtimeMapType)
	mapType = llvm.ConstInsertValue(mapType, rtype, []uint32{0})
	mapType = llvm.ConstInsertValue(mapType, tm.ToRuntime(m.Key), []uint32{1})
	mapType = llvm.ConstInsertValue(mapType, tm.ToRuntime(m.Elt), []uint32{2})
	return tm.makeRuntimeTypeGlobal(mapType)
}

func (tm *TypeMap) chanRuntimeType(c *types.Chan) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(c, reflect.Chan)
	chanType := llvm.ConstNull(tm.runtimeChanType)
	chanType = llvm.ConstInsertValue(chanType, rtype, []uint32{0})
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

func (tm *TypeMap) uncommonType(n *types.NamedType, ptr bool) llvm.Value {
	uncommonTypeInit := llvm.ConstNull(tm.runtimeUncommonType)
	namePtr := tm.globalStringPtr(n.Obj.Name)
	uncommonTypeInit = llvm.ConstInsertValue(uncommonTypeInit, namePtr, []uint32{0})

	// FIXME clean this up
	var pkgpathPtr llvm.Value
	pkgpath := tm.functions.pkgmap[n.Obj]
	if pkgpath != "" {
		pkgpathPtr = tm.globalStringPtr(pkgpath)
		uncommonTypeInit = llvm.ConstInsertValue(uncommonTypeInit, pkgpathPtr, []uint32{1})
	}

	// Store methods.
	// TODO Speed up by sorting objects, rather than names.
	var methods []llvm.Value
	if namedTypeScope, ok := n.Obj.Data.(*ast.Scope); ok {
		methodNames := make([]string, 0, len(namedTypeScope.Objects))
		for name := range namedTypeScope.Objects {
			methodNames = append(methodNames, name)
		}
		sort.Strings(methodNames)
		methods = make([]llvm.Value, 0, len(namedTypeScope.Objects))
		for _, methodName := range methodNames {
			m := namedTypeScope.Lookup(methodName)
			ftyp := m.Type.(*types.Signature)
			ptrrecv := !isIdentical(ftyp.Recv.Type.(types.Type), n)
			if !ptr && ptrrecv {
				// For a type T, we only store methods where the
				// receiver is T and not *T. For *T we store both.
				continue
			}

			method := llvm.ConstNull(tm.runtimeMethod)
			name := tm.globalStringPtr(m.Name)
			name = llvm.ConstBitCast(name, tm.runtimeMethod.StructElementTypes()[0])
			// name
			method = llvm.ConstInsertValue(method, name, []uint32{0})
			// pkgPath
			method = llvm.ConstInsertValue(method, pkgpathPtr, []uint32{1})
			// mtyp (method type, no receiver)
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

			// tfn (standard method/function pointer for plain method calls)
			tfn := tm.resolver.Resolve(m).LLVMValue()
			tfn = llvm.ConstPtrToInt(tfn, tm.target.IntPtrType())

			// ifn (single-word receiver function pointer for interface calls)
			ifn := tfn
			needload := ptr && !ptrrecv
			if !needload {
				recvtyp := tm.ToLLVM(ftyp.Recv.Type.(types.Type))
				needload = int(tm.target.TypeAllocSize(recvtyp)) > tm.target.PointerSize()
			}
			if needload {
				// If the receiver type is wider than a word, we
				// need to use an intermediate function which takes
				// a pointer-receiver, loads it, and then calls the
				// standard receiver function.

				// TODO consolidate this with the one in
				// VisitFuncProtoDecl. Or change the way
				// this is done altogether...
				fname := m.Name
				recvtyp := ftyp.Recv.Type.(types.Type)
				var recvname string
				switch recvtyp := recvtyp.(type) {
				case *types.Pointer:
					named := recvtyp.Base.(*types.NamedType)
					recvname = "*" + named.Obj.Name
				case *types.NamedType:
					recvname = recvtyp.Obj.Name
				}
				fname = fmt.Sprintf("%s.%s", recvname, fname)
				fname = "*" + pkgpath + "." + fname

				ifn = tm.module.NamedFunction(fname)
				ifn = llvm.ConstPtrToInt(ifn, tm.target.IntPtrType())
			}

			method = llvm.ConstInsertValue(method, ifn, []uint32{4})
			method = llvm.ConstInsertValue(method, tfn, []uint32{5})
			methods = append(methods, method)
		}
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
	len_ := llvm.ConstInt(tm.inttype, uint64(len(methods)), false)
	methodsSliceType := tm.runtimeUncommonType.StructElementTypes()[2]
	methodsSlice := llvm.ConstNull(methodsSliceType)
	methodsSlice = llvm.ConstInsertValue(methodsSlice, methodsGlobalPtr, []uint32{0})
	methodsSlice = llvm.ConstInsertValue(methodsSlice, len_, []uint32{1})
	methodsSlice = llvm.ConstInsertValue(methodsSlice, len_, []uint32{2})
	uncommonTypeInit = llvm.ConstInsertValue(uncommonTypeInit, methodsSlice, []uint32{2})
	return uncommonTypeInit
}

func (tm *TypeMap) nameRuntimeType(n *types.NamedType) (global, ptr llvm.Value) {
	pkgpath := tm.functions.pkgmap[n.Obj]
	if pkgpath == "" {
		// Set to "runtime", so the builtin types have a home.
		pkgpath = "runtime"
	}
	globalname := "__llgo.type." + pkgpath + "." + n.Obj.Name
	if pkgpath != tm.pkgpath {
		// We're not compiling the package from whence the type came,
		// so we'll just create a pointer to it here.
		global := llvm.AddGlobal(tm.module, tm.runtimeType, globalname)
		global.SetInitializer(llvm.ConstNull(tm.runtimeType))
		global.SetLinkage(llvm.CommonLinkage)
		return global, global
	}

	underlying := n.Underlying
	if name, ok := underlying.(*types.NamedType); ok {
		underlying = name.Underlying
	}

	global, ptr = tm.makeRuntimeType(underlying)

	// Locate the rtype.
	underlyingRuntimeType := global.Initializer()
	rtype := underlyingRuntimeType
	if rtype.Type() != tm.runtimeType {
		rtype = llvm.ConstExtractValue(rtype, []uint32{0})
	}

	// Insert the uncommon type.
	uncommonTypeInit := tm.uncommonType(n, false)
	uncommonType := llvm.AddGlobal(tm.module, uncommonTypeInit.Type(), "")
	uncommonType.SetInitializer(uncommonTypeInit)
	rtype = llvm.ConstInsertValue(rtype, uncommonType, []uint32{9})

	// Replace the rtype's string representation with the one from
	// uncommonType. XXX should we have the package name prepended? Probably.
	namePtr := llvm.ConstExtractValue(uncommonTypeInit, []uint32{0})
	rtype = llvm.ConstInsertValue(rtype, namePtr, []uint32{8})

	// Update the global's initialiser. Note that we take a copy
	// of the underlying type; we're not updating a shared type.
	if underlyingRuntimeType.Type() != tm.runtimeType {
		underlyingRuntimeType = llvm.ConstInsertValue(underlyingRuntimeType, rtype, []uint32{0})
	} else {
		underlyingRuntimeType = rtype
	}
	global.SetName(globalname)
	global.SetInitializer(underlyingRuntimeType)
	return global, ptr
}

// globalStringPtr returns a *string with the specified value.
func (tm *TypeMap) globalStringPtr(value string) llvm.Value {
	strval := llvm.ConstString(value, false)
	strglobal := llvm.AddGlobal(tm.module, strval.Type(), "")
	strglobal.SetInitializer(strval)
	strglobal = llvm.ConstBitCast(strglobal, llvm.PointerType(llvm.Int8Type(), 0))
	strlen := llvm.ConstInt(tm.inttype, uint64(len(value)), false)
	str := llvm.ConstStruct([]llvm.Value{strglobal, strlen}, false)
	g := llvm.AddGlobal(tm.module, str.Type(), "")
	g.SetInitializer(str)
	return g
}
