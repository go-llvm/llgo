// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/exact"
	"code.google.com/p/go.tools/go/types"
	"fmt"
	"github.com/greggoryhz/gollvm/llvm"
	"go/ast"
	"reflect"
)

type ExprTypeInfo struct {
	types.Type

	// Constant value if non-nil.
	Value exact.Value
}

type ExprTypeMap map[ast.Expr]ExprTypeInfo

type LLVMTypeMap struct {
	TypeStringer
	target  llvm.TargetData
	inttype llvm.Type

	// ptrstandin is a type used to represent the base of a
	// recursive pointer. See llgo/builder.go for how it is used
	// in CreateStore and CreateLoad.
	ptrstandin llvm.Type

	types map[string]llvm.Type // compile-time LLVM type
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

func NewLLVMTypeMap(target llvm.TargetData) *LLVMTypeMap {
	// spec says int is either 32-bit or 64-bit.
	var inttype llvm.Type
	if target.PointerSize() >= 8 {
		inttype = llvm.Int64Type()
	} else {
		inttype = llvm.Int32Type()
	}
	return &LLVMTypeMap{
		TypeStringer: TypeStringer{make(map[*types.TypeName]*types.Package)},
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
		obj := pkg.Scope().Lookup(nil, name)
		if obj == nil {
			panic(fmt.Errorf("Failed to find type: %s", name))
		}
		return tm.ToLLVM(obj.Type())
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
		info.global, info.dyntyp = tm.makeRuntimeType(tstr, t)
		if info.dyntyp.IsNil() {
			panic(fmt.Sprint("Failed to create runtime type for: ", tstr))
		}
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
		return tm.funcLLVMType(tstr, t)
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
	case *types.Named:
		lt := tm.nameLLVMType(t)
		tm.types[tstr] = lt
		return lt
	}
	panic(fmt.Errorf("unhandled: %T", t))
}

func (tm *LLVMTypeMap) basicLLVMType(b *types.Basic) llvm.Type {
	switch b.Kind() {
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
	return llvm.ArrayType(tm.ToLLVM(a.Elem()), int(a.Len()))
}

func (tm *LLVMTypeMap) sliceLLVMType(tstr string, s *types.Slice) llvm.Type {
	typ, ok := tm.types[tstr]
	if !ok {
		typ = llvm.GlobalContext().StructCreateNamed("")
		tm.types[tstr] = typ
		elements := []llvm.Type{
			llvm.PointerType(tm.ToLLVM(s.Elem()), 0),
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
		elements := make([]llvm.Type, s.NumFields())
		for i := range elements {
			f := s.Field(i)
			ft := f.Type()
			elements[i] = tm.ToLLVM(ft)
		}
		typ.StructSetBody(elements, false)
	}
	return typ
}

func (tm *LLVMTypeMap) pointerLLVMType(p *types.Pointer) llvm.Type {
	if p.Elem().Underlying() == p {
		// Recursive pointers must be handled specially, as
		// LLVM does not permit recursive types except via
		// named structs.
		if tm.ptrstandin.IsNil() {
			ctx := llvm.GlobalContext()
			unique := ctx.StructCreateNamed("")
			tm.ptrstandin = llvm.PointerType(unique, 0)
		}
		return llvm.PointerType(tm.ptrstandin, 0)
	}
	return llvm.PointerType(tm.ToLLVM(p.Elem()), 0)
}

func (tm *LLVMTypeMap) funcLLVMType(tstr string, f *types.Signature) llvm.Type {
	typ, ok := tm.types[tstr]
	if !ok {
		// If there's a receiver change the receiver to an
		// additional (first) parameter, and take the value of
		// the resulting signature instead.
		var param_types []llvm.Type
		if recv := f.Recv(); recv != nil {
			params := f.Params()
			paramvars := make([]*types.Var, int(params.Len()+1))
			paramvars[0] = recv
			for i := 0; i < int(params.Len()); i++ {
				paramvars[i+1] = params.At(i)
			}
			params = types.NewTuple(paramvars...)
			f := types.NewSignature(nil, params, f.Results(), f.IsVariadic())
			return tm.ToLLVM(f)
		}

		typ = llvm.GlobalContext().StructCreateNamed("")
		tm.types[tstr] = typ

		params := f.Params()
		nparams := int(params.Len())
		for i := 0; i < nparams; i++ {
			typ := params.At(i).Type()
			if f.IsVariadic() && i == nparams-1 {
				typ = types.NewSlice(typ)
			}
			llvmtyp := tm.ToLLVM(typ)
			param_types = append(param_types, llvmtyp)
		}

		var return_type llvm.Type
		results := f.Results()
		switch nresults := int(results.Len()); nresults {
		case 0:
			return_type = llvm.VoidType()
		case 1:
			return_type = tm.ToLLVM(results.At(0).Type())
		default:
			elements := make([]llvm.Type, nresults)
			for i := range elements {
				result := results.At(i)
				elements[i] = tm.ToLLVM(result.Type())
			}
			return_type = llvm.StructType(elements, false)
		}

		fntyp := llvm.FunctionType(return_type, param_types, false)
		fnptrtyp := llvm.PointerType(fntyp, 0)
		i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
		elements := []llvm.Type{fnptrtyp, i8ptr} // func, closure
		typ.StructSetBody(elements, false)
	}
	return typ
}

func (tm *LLVMTypeMap) interfaceLLVMType(tstr string, i *types.Interface) llvm.Type {
	typ, ok := tm.types[tstr]
	if !ok {
		typ = llvm.GlobalContext().StructCreateNamed("")
		tm.types[tstr] = typ
		valptr_type := llvm.PointerType(llvm.Int8Type(), 0)
		typptr_type := valptr_type // runtimeType may not be defined yet
		elements := make([]llvm.Type, 2+i.NumMethods())
		elements[0] = typptr_type // type
		elements[1] = valptr_type // value
		for n, m := range sortedMethods(i) {
			fntype := m.Type()
			elements[n+2] = tm.ToLLVM(fntype).StructElementTypes()[0]
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

func (tm *LLVMTypeMap) nameLLVMType(n *types.Named) llvm.Type {
	return tm.ToLLVM(n.Underlying())
}

func (tm *LLVMTypeMap) Alignof(typ types.Type) int64 {
	switch typ := typ.Underlying().(type) {
	case *types.Array:
		return tm.Alignof(typ.Elem())
	case *types.Basic:
		switch typ.Kind() {
		case types.Int, types.Uint, types.Int64, types.Uint64,
			types.Float64, types.Complex64, types.Complex128:
			return int64(tm.target.TypeAllocSize(tm.inttype))
		case types.Uintptr, types.UnsafePointer, types.String:
			return int64(tm.target.PointerSize())
		}
		return types.DefaultAlignof(typ)
	case *types.Struct:
		max := int64(1)
		for i := 0; i < typ.NumFields(); i++ {
			f := typ.Field(i)
			a := tm.Alignof(f.Type())
			if a > max {
				max = a
			}
		}
		return max
	}
	return int64(tm.target.PointerSize())
}

func (tm *LLVMTypeMap) Sizeof(typ types.Type) int64 {
	switch typ := typ.Underlying().(type) {
	case *types.Basic:
		switch typ.Kind() {
		case types.Int, types.Uint:
			return int64(tm.target.TypeAllocSize(tm.inttype))
		case types.Uintptr, types.UnsafePointer:
			return int64(tm.target.PointerSize())
		case types.String:
			return 2 * int64(tm.target.PointerSize())
		}
		return types.DefaultSizeof(typ)
	case *types.Array:
		eltsize := tm.Sizeof(typ.Elem())
		eltalign := tm.Alignof(typ.Elem())
		var eltpad int64
		if eltsize%eltalign != 0 {
			eltpad = eltalign - (eltsize % eltalign)
		}
		return (eltsize + eltpad) * typ.Len()
	case *types.Struct:
		n := typ.NumFields()
		if n == 0 {
			return 0
		}
		fields := make([]*types.Field, n)
		for i := range fields {
			fields[i] = typ.Field(i)
		}
		offsets := tm.Offsetsof(fields)
		return offsets[n-1] + tm.Sizeof(fields[n-1].Type())
	}
	return int64(tm.target.PointerSize())
}

func (tm *LLVMTypeMap) Offsetsof(fields []*types.Field) []int64 {
	offsets := make([]int64, len(fields))
	var offset int64
	for i, f := range fields {
		falign := tm.Alignof(f.Type())
		fsize := tm.Sizeof(f.Type())
		if offset%falign != 0 {
			offset += falign - (offset % falign)
		}
		offsets[i] = offset
		offset += fsize
	}
	return offsets
}

///////////////////////////////////////////////////////////////////////////////

func (tm *TypeMap) makeRuntimeType(tstr string, t types.Type) (global, ptr llvm.Value) {
	var info runtimeTypeInfo
	switch t := t.(type) {
	case *types.Basic:
		info.global, info.dyntyp = tm.basicRuntimeType(t)
	case *types.Array:
		info.global, info.dyntyp = tm.arrayRuntimeType(t)
	case *types.Slice:
		info.global, info.dyntyp = tm.sliceRuntimeType(t)
	case *types.Struct:
		return tm.structRuntimeType(tstr, t)
	case *types.Pointer:
		info.global, info.dyntyp = tm.pointerRuntimeType(t)
	case *types.Signature:
		info.global, info.dyntyp = tm.funcRuntimeType(t)
	case *types.Interface:
		info.global, info.dyntyp = tm.interfaceRuntimeType(t)
	case *types.Map:
		info.global, info.dyntyp = tm.mapRuntimeType(t)
	case *types.Chan:
		info.global, info.dyntyp = tm.chanRuntimeType(t)
	case *types.Named:
		info.global, info.dyntyp = tm.nameRuntimeType(t)
	default:
		panic("unreachable")
	}
	tm.types[tstr] = info
	return info.global, info.dyntyp
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
	typ := llvm.ConstNull(tm.runtimeType)
	elementTypes := tm.runtimeType.StructElementTypes()

	// Size.
	size := llvm.ConstInt(elementTypes[0], uint64(tm.Sizeof(t)), false)
	typ = llvm.ConstInsertValue(typ, size, []uint32{0})

	// TODO hash
	// TODO padding

	// Alignment.
	align := llvm.ConstInt(llvm.Int8Type(), uint64(tm.Alignof(t)), false)
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
	rtype := tm.makeRtype(b, basicReflectKinds[b.Kind()])
	return tm.makeRuntimeTypeGlobal(rtype)
}

func (tm *TypeMap) arrayRuntimeType(a *types.Array) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(a, reflect.Array)
	elemRuntimeType := tm.ToRuntime(a.Elem())
	sliceRuntimeType := tm.ToRuntime(types.NewSlice(a.Elem()))
	uintptrlen := llvm.ConstInt(tm.target.IntPtrType(), uint64(a.Len()), false)
	arrayType := llvm.ConstNull(tm.runtimeArrayType)
	arrayType = llvm.ConstInsertValue(arrayType, rtype, []uint32{0})
	arrayType = llvm.ConstInsertValue(arrayType, elemRuntimeType, []uint32{1})
	arrayType = llvm.ConstInsertValue(arrayType, sliceRuntimeType, []uint32{2})
	arrayType = llvm.ConstInsertValue(arrayType, uintptrlen, []uint32{3})
	return tm.makeRuntimeTypeGlobal(arrayType)
}

func (tm *TypeMap) sliceRuntimeType(s *types.Slice) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(s, reflect.Slice)
	elemRuntimeType := tm.ToRuntime(s.Elem())
	sliceType := llvm.ConstNull(tm.runtimeSliceType)
	sliceType = llvm.ConstInsertValue(sliceType, rtype, []uint32{0})
	sliceType = llvm.ConstInsertValue(sliceType, elemRuntimeType, []uint32{1})
	return tm.makeRuntimeTypeGlobal(sliceType)
}

func (tm *TypeMap) structRuntimeType(tstr string, s *types.Struct) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(s, reflect.Struct)
	structType := llvm.ConstNull(tm.runtimeStructType)
	structType = llvm.ConstInsertValue(structType, rtype, []uint32{0})
	global, ptr = tm.makeRuntimeTypeGlobal(structType)
	// TODO set fields, reset initialiser
	return
}

func (tm *TypeMap) pointerRuntimeType(p *types.Pointer) (global, ptr llvm.Value) {
	// Is the base type a named type from another package? If so, we'll
	// create a reference to the externally defined symbol.
	var globalname string
	if n, ok := p.Elem().(*types.Named); ok {
		// FIXME horrible circular relationship
		var path string
		if data, ok := tm.functions.objectdata[n.Obj()]; ok {
			path = pkgpath(data.Package)
		}
		if path == "" {
			path = "runtime"
		}
		globalname = "__llgo.type.*" + path + "." + n.Obj().Name()
		if path != tm.pkgpath {
			global := llvm.AddGlobal(tm.module, tm.runtimeType, globalname)
			global.SetInitializer(llvm.ConstNull(tm.runtimeType))
			global.SetLinkage(llvm.CommonLinkage)
			return global, global
		}
	}

	rtype := tm.makeRtype(p, reflect.Ptr)
	if n, ok := p.Elem().(*types.Named); ok {
		uncommonTypeInit := tm.uncommonType(n, true)
		uncommonType := llvm.AddGlobal(tm.module, uncommonTypeInit.Type(), "")
		uncommonType.SetInitializer(uncommonTypeInit)
		rtype = llvm.ConstInsertValue(rtype, uncommonType, []uint32{9})
	}

	ptrType := llvm.ConstNull(tm.runtimePtrType)
	var baseTypeGlobal llvm.Value
	if p.Elem().Underlying() == p {
		// Recursive pointer.
		ptrType = llvm.ConstInsertValue(ptrType, rtype, []uint32{0})
		global, ptr = tm.makeRuntimeTypeGlobal(ptrType)
		baseTypeGlobal = global

		// Update the global with its own pointer in the elem field.
		ptrType = global.Initializer()
		ptrType = llvm.ConstInsertValue(ptrType, ptr, []uint32{1})
		global.SetInitializer(ptrType)
	} else {
		var baseTypePtr llvm.Value
		baseTypeGlobal, baseTypePtr = tm.toRuntime(p.Elem())
		ptrType = llvm.ConstInsertValue(ptrType, rtype, []uint32{0})
		ptrType = llvm.ConstInsertValue(ptrType, baseTypePtr, []uint32{1})
		global, ptr = tm.makeRuntimeTypeGlobal(ptrType)
	}
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
	if f.IsVariadic() {
		variadic := llvm.ConstInt(llvm.Int1Type(), 1, false)
		funcType = llvm.ConstInsertValue(funcType, variadic, []uint32{1})
	}
	// TODO in
	//funcType = llvm.ConstInsertValue(funcType, tm.ToRuntime(p.Elt()), []uint32{2})
	// TODO out
	//funcType = llvm.ConstInsertValue(funcType, tm.ToRuntime(p.Elt()), []uint32{3})
	return tm.makeRuntimeTypeGlobal(funcType)
}

func (tm *TypeMap) interfaceRuntimeType(i *types.Interface) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(i, reflect.Interface)
	interfaceType := llvm.ConstNull(tm.runtimeInterfaceType)
	interfaceType = llvm.ConstInsertValue(interfaceType, rtype, []uint32{0})

	imethods := make([]llvm.Value, i.NumMethods())
	for index, method := range sortedMethods(i) {
		//name, pkgPath, type
		imethod := llvm.ConstNull(tm.runtimeImethod)
		name := tm.globalStringPtr(method.Name())
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

	len_ := llvm.ConstInt(tm.inttype, uint64(i.NumMethods()), false)
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
	mapType = llvm.ConstInsertValue(mapType, tm.ToRuntime(m.Key()), []uint32{1})
	mapType = llvm.ConstInsertValue(mapType, tm.ToRuntime(m.Elem()), []uint32{2})
	return tm.makeRuntimeTypeGlobal(mapType)
}

func (tm *TypeMap) chanRuntimeType(c *types.Chan) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(c, reflect.Chan)
	chanType := llvm.ConstNull(tm.runtimeChanType)
	chanType = llvm.ConstInsertValue(chanType, rtype, []uint32{0})
	chanType = llvm.ConstInsertValue(chanType, tm.ToRuntime(c.Elem()), []uint32{1})

	// go/ast and reflect disagree on values for direction.
	var dir reflect.ChanDir
	if c.Dir()&ast.SEND != 0 {
		dir = reflect.SendDir
	}
	if c.Dir()&ast.RECV != 0 {
		dir |= reflect.RecvDir
	}
	uintptrdir := llvm.ConstInt(tm.target.IntPtrType(), uint64(dir), false)
	chanType = llvm.ConstInsertValue(chanType, uintptrdir, []uint32{2})
	return tm.makeRuntimeTypeGlobal(chanType)
}

func (tm *TypeMap) uncommonType(n *types.Named, ptr bool) llvm.Value {
	uncommonTypeInit := llvm.ConstNull(tm.runtimeUncommonType)
	namePtr := tm.globalStringPtr(n.Obj().Name())
	uncommonTypeInit = llvm.ConstInsertValue(uncommonTypeInit, namePtr, []uint32{0})

	// FIXME clean this up
	var pkgpathPtr llvm.Value
	var path string
	if data, ok := tm.functions.objectdata[n.Obj()]; ok {
		path = pkgpath(data.Package)
	}
	if path != "" {
		pkgpathPtr = tm.globalStringPtr(path)
		uncommonTypeInit = llvm.ConstInsertValue(uncommonTypeInit, pkgpathPtr, []uint32{1})
	}

	methodset := tm.functions.methods(n)
	methodfuncs := methodset.nonptr
	if ptr {
		methodfuncs = methodset.ptr
	}

	// Store methods.
	methods := make([]llvm.Value, len(methodfuncs))
	for i, mfunc := range methodfuncs {
		ftyp := mfunc.Type().(*types.Signature)

		method := llvm.ConstNull(tm.runtimeMethod)
		name := tm.globalStringPtr(mfunc.Name())
		name = llvm.ConstBitCast(name, tm.runtimeMethod.StructElementTypes()[0])
		// name
		method = llvm.ConstInsertValue(method, name, []uint32{0})
		// pkgPath
		method = llvm.ConstInsertValue(method, pkgpathPtr, []uint32{1})
		// mtyp (method type, no receiver)
		{
			ftyp := types.NewSignature(nil, ftyp.Params(), ftyp.Results(), ftyp.IsVariadic())
			mtyp := tm.ToRuntime(ftyp)
			method = llvm.ConstInsertValue(method, mtyp, []uint32{2})
		}
		// typ (function type, with receiver)
		typ := tm.ToRuntime(ftyp)
		method = llvm.ConstInsertValue(method, typ, []uint32{3})

		// tfn (standard method/function pointer for plain method calls)
		tfn := tm.resolver.Resolve(tm.functions.objectdata[mfunc].Ident).LLVMValue()
		tfn = llvm.ConstExtractValue(tfn, []uint32{0})
		tfn = llvm.ConstPtrToInt(tfn, tm.target.IntPtrType())

		// ifn (single-word receiver function pointer for interface calls)
		ifn := tfn
		if !ptr && tm.Sizeof(ftyp.Recv().Type()) > int64(tm.target.PointerSize()) {
			mfunc := methodset.lookup(mfunc.Name(), true)
			ifn = tm.resolver.Resolve(tm.functions.objectdata[mfunc].Ident).LLVMValue()
			ifn = llvm.ConstExtractValue(ifn, []uint32{0})
			ifn = llvm.ConstPtrToInt(ifn, tm.target.IntPtrType())
		}

		method = llvm.ConstInsertValue(method, ifn, []uint32{4})
		method = llvm.ConstInsertValue(method, tfn, []uint32{5})
		methods[i] = method
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

func (tm *TypeMap) nameRuntimeType(n *types.Named) (global, ptr llvm.Value) {
	var path string
	if data, ok := tm.functions.objectdata[n.Obj()]; ok {
		path = pkgpath(data.Package)
	}
	if path == "" {
		// Set to "runtime", so the builtin types have a home.
		path = "runtime"
	}
	globalname := "__llgo.type." + path + "." + n.Obj().Name()
	if path != tm.pkgpath {
		// We're not compiling the package from whence the type came,
		// so we'll just create a pointer to it here.
		global := llvm.AddGlobal(tm.module, tm.runtimeType, globalname)
		global.SetInitializer(llvm.ConstNull(tm.runtimeType))
		global.SetLinkage(llvm.CommonLinkage)
		return global, global
	}

	underlying := n.Underlying()
	if name, ok := underlying.(*types.Named); ok {
		underlying = name.Underlying()
	}

	global, ptr = tm.toRuntime(underlying)

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
