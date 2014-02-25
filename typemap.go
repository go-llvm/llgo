// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"fmt"
	"go/ast"
	"reflect"

	"code.google.com/p/go.tools/go/types"
	"code.google.com/p/go.tools/go/types/typeutil"
	"github.com/axw/gollvm/llvm"
)

type MethodResolver interface {
	ResolveMethod(*types.Selection) *LLVMValue
}

// llvmTypeMap is provides a means of mapping from a types.Map
// to llgo's corresponding LLVM type representation.
type llvmTypeMap struct {
	*types.StdSizes
	target  llvm.TargetData
	inttype llvm.Type

	// ptrstandin is a type used to represent the base of a
	// recursive pointer. See llgo/builder.go for how it is used
	// in CreateStore and CreateLoad.
	ptrstandin llvm.Type

	types typeutil.Map
}

type runtimeTypeInfo struct {
	global llvm.Value
	dyntyp llvm.Value
}

type TypeMap struct {
	*llvmTypeMap

	module         llvm.Module
	pkgpath        string
	types          typeutil.Map
	runtime        *runtimeInterface
	methodResolver MethodResolver
	alg            *algorithmMap
	types.MethodSetCache

	hashAlgFunctionType,
	equalAlgFunctionType,
	printAlgFunctionType,
	copyAlgFunctionType llvm.Type
}

func NewLLVMTypeMap(target llvm.TargetData) *llvmTypeMap {
	// spec says int is either 32-bit or 64-bit.
	var inttype llvm.Type
	if target.PointerSize() >= 8 {
		inttype = llvm.Int64Type()
	} else {
		inttype = llvm.Int32Type()
	}
	return &llvmTypeMap{
		StdSizes: &types.StdSizes{
			WordSize: int64(target.PointerSize()),
			MaxAlign: 8,
		},
		target:     target,
		inttype:    inttype,
		ptrstandin: llvm.GlobalContext().StructCreateNamed(""),
	}
}

func NewTypeMap(pkgpath string, llvmtm *llvmTypeMap, module llvm.Module, r *runtimeInterface, mr MethodResolver) *TypeMap {
	return &TypeMap{
		llvmTypeMap:    llvmtm,
		module:         module,
		pkgpath:        pkgpath,
		runtime:        r,
		methodResolver: mr,
		alg:            newAlgorithmMap(module, r, llvmtm.target),
	}
}

func (tm *llvmTypeMap) ToLLVM(t types.Type) llvm.Type {
	return tm.toLLVM(t, "")
}

func (tm *llvmTypeMap) toLLVM(t types.Type, name string) llvm.Type {
	// Signature needs to be handled specially, to preprocess
	// methods, moving the receiver to the parameter list.
	if t, ok := t.(*types.Signature); ok {
		return tm.funcLLVMType(t, name)
	}
	lt, ok := tm.types.At(t).(llvm.Type)
	if !ok {
		lt = tm.makeLLVMType(t, name)
		if lt.IsNil() {
			panic(fmt.Sprint("Failed to create LLVM type for: ", t))
		}
		tm.types.Set(t, lt)
	}
	return lt
}

// ToRuntime returns a pointer to the specified type's runtime type descriptor.
func (tm *TypeMap) ToRuntime(t types.Type) llvm.Value {
	_, r := tm.toRuntime(t)
	return r
}

func (tm *TypeMap) toRuntime(t types.Type) (global, value llvm.Value) {
	info, ok := tm.types.At(t).(runtimeTypeInfo)
	if !ok {
		info.global, info.dyntyp = tm.makeRuntimeType(t)
		if info.dyntyp.IsNil() {
			panic(fmt.Sprint("Failed to create runtime type for: ", t))
		}
	}
	return info.global, info.dyntyp
}

func (tm *llvmTypeMap) makeLLVMType(t types.Type, name string) llvm.Type {
	switch t := t.(type) {
	case *types.Basic:
		return tm.basicLLVMType(t)
	case *types.Array:
		return tm.arrayLLVMType(t)
	case *types.Slice:
		return tm.sliceLLVMType(t, name)
	case *types.Struct:
		return tm.structLLVMType(t, name)
	case *types.Pointer:
		return tm.pointerLLVMType(t)
	case *types.Interface:
		return tm.interfaceLLVMType(t, name)
	case *types.Map:
		return tm.mapLLVMType(t)
	case *types.Chan:
		return tm.chanLLVMType(t)
	case *types.Named:
		// First we set ptrstandin, in case we've got a recursive pointer.
		if _, ok := t.Underlying().(*types.Pointer); ok {
			tm.types.Set(t, tm.ptrstandin)
		}
		return tm.nameLLVMType(t)
	}
	panic(fmt.Errorf("unhandled: %T", t))
}

func (tm *llvmTypeMap) basicLLVMType(b *types.Basic) llvm.Type {
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

func (tm *llvmTypeMap) arrayLLVMType(a *types.Array) llvm.Type {
	return llvm.ArrayType(tm.ToLLVM(a.Elem()), int(a.Len()))
}

func (tm *llvmTypeMap) sliceLLVMType(s *types.Slice, name string) llvm.Type {
	typ, ok := tm.types.At(s).(llvm.Type)
	if !ok {
		typ = llvm.GlobalContext().StructCreateNamed(name)
		tm.types.Set(s, typ)
		elements := []llvm.Type{
			llvm.PointerType(tm.ToLLVM(s.Elem()), 0),
			tm.inttype,
			tm.inttype,
		}
		typ.StructSetBody(elements, false)
	}
	return typ
}

func (tm *llvmTypeMap) structLLVMType(s *types.Struct, name string) llvm.Type {
	typ, ok := tm.types.At(s).(llvm.Type)
	if !ok {
		typ = llvm.GlobalContext().StructCreateNamed(name)
		tm.types.Set(s, typ)
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

func (tm *llvmTypeMap) pointerLLVMType(p *types.Pointer) llvm.Type {
	return llvm.PointerType(tm.ToLLVM(p.Elem()), 0)
}

func (tm *llvmTypeMap) funcLLVMType(f *types.Signature, name string) llvm.Type {
	// If there's a receiver change the receiver to an
	// additional (first) parameter, and take the value of
	// the resulting signature instead.
	if recv := f.Recv(); recv != nil {
		params := f.Params()
		paramvars := make([]*types.Var, int(params.Len()+1))
		paramvars[0] = recv
		for i := 0; i < int(params.Len()); i++ {
			paramvars[i+1] = params.At(i)
		}
		params = types.NewTuple(paramvars...)
		f := types.NewSignature(nil, nil, params, f.Results(), f.Variadic())
		return tm.toLLVM(f, name)
	}

	if typ, ok := tm.types.At(f).(llvm.Type); ok {
		return typ
	}
	typ := llvm.GlobalContext().StructCreateNamed(name)
	tm.types.Set(f, typ)

	params := f.Params()
	param_types := make([]llvm.Type, params.Len())
	for i := range param_types {
		llvmtyp := tm.ToLLVM(params.At(i).Type())
		param_types[i] = llvmtyp
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
	return typ
}

func (tm *llvmTypeMap) interfaceLLVMType(i *types.Interface, name string) llvm.Type {
	if typ, ok := tm.types.At(i).(llvm.Type); ok {
		return typ
	}
	// interface{} is represented as {type, value},
	// and non-empty interfaces are represented as {itab, value}.
	i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
	rtypeType := i8ptr
	valueType := i8ptr
	if name == "" {
		name = i.String()
	}
	typ := llvm.GlobalContext().StructCreateNamed(name)
	typ.StructSetBody([]llvm.Type{rtypeType, valueType}, false)
	return typ
}

func (tm *llvmTypeMap) mapLLVMType(m *types.Map) llvm.Type {
	// All map details are in the runtime. We represent it here as an
	// opaque pointer.
	return tm.target.IntPtrType()
}

func (tm *llvmTypeMap) chanLLVMType(c *types.Chan) llvm.Type {
	// All channel details are in the runtime. We represent it
	// here as an opaque pointer.
	return tm.target.IntPtrType()
}

func (tm *llvmTypeMap) nameLLVMType(n *types.Named) llvm.Type {
	return tm.toLLVM(n.Underlying(), n.String())
}

func (tm *llvmTypeMap) Alignof(typ types.Type) int64 {
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
		return tm.StdSizes.Alignof(typ)
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

func (tm *llvmTypeMap) Sizeof(typ types.Type) int64 {
	switch typ := typ.Underlying().(type) {
	case *types.Basic:
		switch typ.Kind() {
		case types.Int, types.Uint:
			return int64(tm.target.TypeAllocSize(tm.inttype))
		case types.Uintptr, types.UnsafePointer:
			return int64(tm.target.PointerSize())
		}
		return tm.StdSizes.Sizeof(typ)
	case *types.Array:
		eltsize := tm.Sizeof(typ.Elem())
		eltalign := tm.Alignof(typ.Elem())
		var eltpad int64
		if eltsize%eltalign != 0 {
			eltpad = eltalign - (eltsize % eltalign)
		}
		return (eltsize + eltpad) * typ.Len()
	case *types.Slice:
		return 3 * int64(tm.target.PointerSize())
	case *types.Struct:
		n := typ.NumFields()
		if n == 0 {
			return 0
		}
		fields := make([]*types.Var, n)
		for i := range fields {
			fields[i] = typ.Field(i)
		}
		offsets := tm.Offsetsof(fields)
		return offsets[n-1] + tm.Sizeof(fields[n-1].Type())
	case *types.Interface:
		return int64((2 + typ.NumMethods()) * tm.target.PointerSize())
	}
	return int64(tm.target.PointerSize())
}

func (tm *llvmTypeMap) Offsetsof(fields []*types.Var) []int64 {
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

func (tm *TypeMap) makeRuntimeType(t types.Type) (global, ptr llvm.Value) {
	switch t := t.(type) {
	case *types.Basic:
		global, ptr = tm.basicRuntimeType(t, false)
	case *types.Array:
		global, ptr = tm.arrayRuntimeType(t)
	case *types.Slice:
		return tm.sliceRuntimeType(t)
	case *types.Struct:
		return tm.structRuntimeType(t)
	case *types.Pointer:
		global, ptr = tm.pointerRuntimeType(t)
	case *types.Signature:
		return tm.funcRuntimeType(t)
	case *types.Interface:
		return tm.interfaceRuntimeType(t)
	case *types.Map:
		global, ptr = tm.mapRuntimeType(t)
	case *types.Chan:
		global, ptr = tm.chanRuntimeType(t)
	case *types.Named:
		global, ptr = tm.nameRuntimeType(t)
	default:
		panic(fmt.Sprintf("unhandled type: %#v", t))
	}
	tm.types.Set(t, runtimeTypeInfo{global, ptr})
	return global, ptr
}

func (tm *TypeMap) makeAlgorithmTable(t types.Type) llvm.Value {
	// TODO set these to actual functions.
	hashAlg := llvm.ConstNull(llvm.PointerType(tm.alg.hashAlgFunctionType, 0))
	printAlg := llvm.ConstNull(llvm.PointerType(tm.alg.printAlgFunctionType, 0))
	copyAlg := llvm.ConstNull(llvm.PointerType(tm.alg.copyAlgFunctionType, 0))
	equalAlg := tm.alg.eqalg(t)
	elems := []llvm.Value{
		AlgorithmHash:  hashAlg,
		AlgorithmEqual: equalAlg,
		AlgorithmPrint: printAlg,
		AlgorithmCopy:  copyAlg,
	}
	return llvm.ConstStruct(elems, false)
}

func (tm *TypeMap) makeRuntimeTypeGlobal(v llvm.Value, name string) (global, ptr llvm.Value) {
	global = llvm.AddGlobal(tm.module, v.Type(), typeSymbol(name))
	global.SetInitializer(v)
	global.SetLinkage(llvm.LinkOnceAnyLinkage)
	ptr = llvm.ConstBitCast(global, llvm.PointerType(tm.runtime.rtype.llvm, 0))
	return global, ptr
}

func (tm *TypeMap) makeRtype(t types.Type, k reflect.Kind) llvm.Value {
	// Not sure if there's an easier way to do this, but if you just
	// use ConstStruct, you end up getting a different llvm.Type.
	typ := llvm.ConstNull(tm.runtime.rtype.llvm)
	elementTypes := tm.runtime.rtype.llvm.StructElementTypes()

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
	stringrep := tm.globalStringPtr(t.String())
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

func typeString(t types.Type) string {
	return types.TypeString(nil, t)
}

func typeSymbol(name string) string {
	if name == "" {
		return ""
	}
	return "__llgo.type." + name
}

// basicRuntimeType creates the runtime type structure for
// a basic type. If underlying is true, then a new global
// is always created.
func (tm *TypeMap) basicRuntimeType(b *types.Basic, underlying bool) (global, ptr llvm.Value) {
	b = types.Typ[b.Kind()] // unalias
	var name string
	if !underlying {
		name = typeString(b)
		if tm.pkgpath != "runtime" {
			global := llvm.AddGlobal(tm.module, tm.runtime.rtype.llvm, typeSymbol(name))
			global.SetInitializer(llvm.ConstNull(tm.runtime.rtype.llvm))
			global.SetLinkage(llvm.CommonLinkage)
			return global, global
		}
	}
	rtype := tm.makeRtype(b, basicReflectKinds[b.Kind()])
	global, ptr = tm.makeRuntimeTypeGlobal(rtype, name)
	global.SetLinkage(llvm.ExternalLinkage)
	if !underlying {
		switch b.Kind() {
		case types.Int32:
			llvm.AddAlias(tm.module, global.Type(), global, typeSymbol("rune"))
		case types.Uint8:
			llvm.AddAlias(tm.module, global.Type(), global, typeSymbol("byte"))
		}
	}
	return global, ptr
}

func (tm *TypeMap) arrayRuntimeType(a *types.Array) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(a, reflect.Array)
	elemRuntimeType := tm.ToRuntime(a.Elem())
	sliceRuntimeType := tm.ToRuntime(types.NewSlice(a.Elem()))
	uintptrlen := llvm.ConstInt(tm.target.IntPtrType(), uint64(a.Len()), false)
	arrayType := llvm.ConstNull(tm.runtime.arrayType.llvm)
	arrayType = llvm.ConstInsertValue(arrayType, rtype, []uint32{0})
	arrayType = llvm.ConstInsertValue(arrayType, elemRuntimeType, []uint32{1})
	arrayType = llvm.ConstInsertValue(arrayType, sliceRuntimeType, []uint32{2})
	arrayType = llvm.ConstInsertValue(arrayType, uintptrlen, []uint32{3})
	return tm.makeRuntimeTypeGlobal(arrayType, typeString(a))
}

func (tm *TypeMap) sliceRuntimeType(s *types.Slice) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(s, reflect.Slice)
	sliceType := llvm.ConstNull(tm.runtime.sliceType.llvm)
	global, ptr = tm.makeRuntimeTypeGlobal(sliceType, typeString(s))
	tm.types.Set(s, runtimeTypeInfo{global, ptr})
	sliceType = llvm.ConstInsertValue(sliceType, rtype, []uint32{0})
	elemRuntimeType := tm.ToRuntime(s.Elem())
	sliceType = llvm.ConstInsertValue(sliceType, elemRuntimeType, []uint32{1})
	global.SetInitializer(sliceType)
	return global, ptr
}

func (tm *TypeMap) structRuntimeType(s *types.Struct) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(s, reflect.Struct)
	structType := llvm.ConstNull(tm.runtime.structType.llvm)
	structType = llvm.ConstInsertValue(structType, rtype, []uint32{0})
	global, ptr = tm.makeRuntimeTypeGlobal(structType, typeString(s))
	tm.types.Set(s, runtimeTypeInfo{global, ptr})
	fieldVars := make([]*types.Var, s.NumFields())
	for i := range fieldVars {
		fieldVars[i] = s.Field(i)
	}
	offsets := tm.Offsetsof(fieldVars)
	structFields := make([]llvm.Value, len(fieldVars))
	for i := range structFields {
		field := fieldVars[i]
		structField := llvm.ConstNull(tm.runtime.structField.llvm)
		if !field.Anonymous() {
			name := tm.globalStringPtr(field.Name())
			name = llvm.ConstBitCast(name, tm.runtime.structField.llvm.StructElementTypes()[0])
			structField = llvm.ConstInsertValue(structField, name, []uint32{0})
		}
		if !ast.IsExported(field.Name()) {
			pkgpath := tm.globalStringPtr(field.Pkg().Path())
			pkgpath = llvm.ConstBitCast(pkgpath, tm.runtime.structField.llvm.StructElementTypes()[1])
			structField = llvm.ConstInsertValue(structField, pkgpath, []uint32{1})
		}
		fieldType := tm.ToRuntime(field.Type())
		structField = llvm.ConstInsertValue(structField, fieldType, []uint32{2})
		if tag := s.Tag(i); tag != "" {
			tag := tm.globalStringPtr(tag)
			tag = llvm.ConstBitCast(tag, tm.runtime.structField.llvm.StructElementTypes()[3])
			structField = llvm.ConstInsertValue(structField, tag, []uint32{3})
		}
		offset := llvm.ConstInt(tm.runtime.structField.llvm.StructElementTypes()[4], uint64(offsets[i]), false)
		structField = llvm.ConstInsertValue(structField, offset, []uint32{4})
		structFields[i] = structField
	}
	structFieldsSliceType := tm.runtime.structType.llvm.StructElementTypes()[1]
	structFieldsSlice := tm.makeSlice(structFields, structFieldsSliceType)
	structType = llvm.ConstInsertValue(structType, structFieldsSlice, []uint32{1})
	global.SetInitializer(structType)
	return global, ptr
}

func (tm *TypeMap) pointerRuntimeType(p *types.Pointer) (global, ptr llvm.Value) {
	// Is the base type a named type from another package? If so, we'll
	// create a reference to the externally defined symbol.
	linkage := llvm.LinkOnceAnyLinkage
	switch elem := p.Elem().(type) {
	case *types.Basic:
		if tm.pkgpath != "runtime" {
			global := llvm.AddGlobal(tm.module, tm.runtime.rtype.llvm, typeSymbol(typeString(p)))
			global.SetInitializer(llvm.ConstNull(tm.runtime.rtype.llvm))
			global.SetLinkage(llvm.CommonLinkage)
			return global, global
		}
		linkage = llvm.ExternalLinkage
	case *types.Named:
		path := "runtime"
		if pkg := elem.Obj().Pkg(); pkg != nil {
			path = pkg.Path()
		}
		if path != tm.pkgpath {
			global := llvm.AddGlobal(tm.module, tm.runtime.rtype.llvm, typeSymbol(typeString(p)))
			global.SetInitializer(llvm.ConstNull(tm.runtime.rtype.llvm))
			global.SetLinkage(llvm.CommonLinkage)
			return global, global
		}
		linkage = llvm.ExternalLinkage
	}

	rtype := tm.makeRtype(p, reflect.Ptr)
	if n, ok := p.Elem().(*types.Named); ok {
		uncommonTypeInit := tm.uncommonType(n, p)
		uncommonType := llvm.AddGlobal(tm.module, uncommonTypeInit.Type(), "")
		uncommonType.SetInitializer(uncommonTypeInit)
		rtype = llvm.ConstInsertValue(rtype, uncommonType, []uint32{9})
	}

	ptrType := llvm.ConstNull(tm.runtime.ptrType.llvm)
	var baseTypeGlobal llvm.Value
	if p.Elem().Underlying() == p {
		// Recursive pointer.
		ptrType = llvm.ConstInsertValue(ptrType, rtype, []uint32{0})
		global, ptr = tm.makeRuntimeTypeGlobal(ptrType, typeString(p))
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
		global, ptr = tm.makeRuntimeTypeGlobal(ptrType, typeString(p))
	}
	global.SetLinkage(linkage)

	// Set ptrToThis in the base type's rtype.
	baseType := baseTypeGlobal.Initializer()
	if !baseType.IsNull() {
		if baseType.Type() == tm.runtime.rtype.llvm {
			baseType = llvm.ConstInsertValue(baseType, ptr, []uint32{10})
		} else {
			rtype := llvm.ConstExtractValue(baseType, []uint32{0})
			rtype = llvm.ConstInsertValue(rtype, ptr, []uint32{10})
			baseType = llvm.ConstInsertValue(baseType, rtype, []uint32{0})
		}
		baseTypeGlobal.SetInitializer(baseType)
	}
	return global, ptr
}

func (tm *TypeMap) rtypeSlice(t *types.Tuple) llvm.Value {
	rtypes := make([]llvm.Value, t.Len())
	for i := range rtypes {
		rtypes[i] = tm.ToRuntime(t.At(i).Type())
	}
	slicetyp := tm.runtime.funcType.llvm.StructElementTypes()[2]
	return tm.makeSlice(rtypes, slicetyp)
}

func (tm *TypeMap) funcRuntimeType(f *types.Signature) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(f, reflect.Func)
	funcType := llvm.ConstNull(tm.runtime.funcType.llvm)
	global, ptr = tm.makeRuntimeTypeGlobal(funcType, typeString(f))
	tm.types.Set(f, runtimeTypeInfo{global, ptr})
	funcType = llvm.ConstInsertValue(funcType, rtype, []uint32{0})
	// dotdotdot
	if f.Variadic() {
		variadic := llvm.ConstInt(llvm.Int1Type(), 1, false)
		funcType = llvm.ConstInsertValue(funcType, variadic, []uint32{1})
	}
	// in
	intypes := tm.rtypeSlice(f.Params())
	funcType = llvm.ConstInsertValue(funcType, intypes, []uint32{2})
	// out
	outtypes := tm.rtypeSlice(f.Results())
	funcType = llvm.ConstInsertValue(funcType, outtypes, []uint32{3})
	global.SetInitializer(funcType)
	return global, ptr
}

func (tm *TypeMap) interfaceRuntimeType(i *types.Interface) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(i, reflect.Interface)
	interfaceType := llvm.ConstNull(tm.runtime.interfaceType.llvm)
	global, ptr = tm.makeRuntimeTypeGlobal(interfaceType, typeString(i))
	tm.types.Set(i, runtimeTypeInfo{global, ptr})
	interfaceType = llvm.ConstInsertValue(interfaceType, rtype, []uint32{0})
	methodset := tm.MethodSet(i)
	imethods := make([]llvm.Value, methodset.Len())
	for index := 0; index < methodset.Len(); index++ {
		method := methodset.At(index).Obj()
		imethod := llvm.ConstNull(tm.runtime.imethod.llvm)
		name := tm.globalStringPtr(method.Name())
		name = llvm.ConstBitCast(name, tm.runtime.imethod.llvm.StructElementTypes()[0])
		mtyp := tm.ToRuntime(method.Type())
		imethod = llvm.ConstInsertValue(imethod, name, []uint32{0})
		if !ast.IsExported(method.Name()) {
			pkgpath := tm.globalStringPtr(method.Pkg().Path())
			pkgpath = llvm.ConstBitCast(pkgpath, tm.runtime.imethod.llvm.StructElementTypes()[1])
			imethod = llvm.ConstInsertValue(imethod, pkgpath, []uint32{1})
		}
		imethod = llvm.ConstInsertValue(imethod, mtyp, []uint32{2})
		imethods[index] = imethod
	}
	imethodsSliceType := tm.runtime.interfaceType.llvm.StructElementTypes()[1]
	imethodsSlice := tm.makeSlice(imethods, imethodsSliceType)
	interfaceType = llvm.ConstInsertValue(interfaceType, imethodsSlice, []uint32{1})
	global.SetInitializer(interfaceType)
	return global, ptr
}

func (tm *TypeMap) mapRuntimeType(m *types.Map) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(m, reflect.Map)
	mapType := llvm.ConstNull(tm.runtime.mapType.llvm)
	mapType = llvm.ConstInsertValue(mapType, rtype, []uint32{0})
	mapType = llvm.ConstInsertValue(mapType, tm.ToRuntime(m.Key()), []uint32{1})
	mapType = llvm.ConstInsertValue(mapType, tm.ToRuntime(m.Elem()), []uint32{2})
	return tm.makeRuntimeTypeGlobal(mapType, typeString(m))
}

func (tm *TypeMap) chanRuntimeType(c *types.Chan) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(c, reflect.Chan)
	chanType := llvm.ConstNull(tm.runtime.chanType.llvm)
	chanType = llvm.ConstInsertValue(chanType, rtype, []uint32{0})
	chanType = llvm.ConstInsertValue(chanType, tm.ToRuntime(c.Elem()), []uint32{1})

	// go/ast and reflect disagree on values for direction.
	var dir reflect.ChanDir
	switch c.Dir() {
	case types.SendOnly:
		dir = reflect.SendDir
	case types.RecvOnly:
		dir = reflect.RecvDir
	case types.SendRecv:
		dir = reflect.SendDir | reflect.RecvDir
	}
	uintptrdir := llvm.ConstInt(tm.target.IntPtrType(), uint64(dir), false)
	chanType = llvm.ConstInsertValue(chanType, uintptrdir, []uint32{2})
	return tm.makeRuntimeTypeGlobal(chanType, typeString(c))
}

// p != nil iff we're generatig the uncommonType for a pointer type.
func (tm *TypeMap) uncommonType(n *types.Named, p *types.Pointer) llvm.Value {
	uncommonTypeInit := llvm.ConstNull(tm.runtime.uncommonType.llvm)
	namePtr := tm.globalStringPtr(n.Obj().Name())
	uncommonTypeInit = llvm.ConstInsertValue(uncommonTypeInit, namePtr, []uint32{0})
	var path string
	if pkg := n.Obj().Pkg(); pkg != nil {
		path = pkg.Path()
	}
	pkgpathPtr := tm.globalStringPtr(path)
	uncommonTypeInit = llvm.ConstInsertValue(uncommonTypeInit, pkgpathPtr, []uint32{1})

	// If we're dealing with an interface, stop now;
	// we store interface methods on the interface
	// type.
	if _, ok := n.Underlying().(*types.Interface); ok {
		return uncommonTypeInit
	}

	var methodset, pmethodset *types.MethodSet
	if p != nil {
		methodset = tm.MethodSet(p)
	} else {
		methodset = tm.MethodSet(n)
	}

	// Store methods. All methods must be stored, not only exported ones;
	// this is to allow satisfying of interfaces with non-exported methods.
	methods := make([]llvm.Value, methodset.Len())
	for i := range methods {
		sel := methodset.At(i)
		mname := sel.Obj().Name()
		mfunc := tm.methodResolver.ResolveMethod(sel)
		ftyp := mfunc.Type().(*types.Signature)

		method := llvm.ConstNull(tm.runtime.method.llvm)
		name := tm.globalStringPtr(mname)
		name = llvm.ConstBitCast(name, tm.runtime.method.llvm.StructElementTypes()[0])
		// name
		method = llvm.ConstInsertValue(method, name, []uint32{0})
		// pkgPath
		method = llvm.ConstInsertValue(method, pkgpathPtr, []uint32{1})
		// mtyp (method type, no receiver)
		{
			ftyp := types.NewSignature(nil, nil, ftyp.Params(), ftyp.Results(), ftyp.Variadic())
			mtyp := tm.ToRuntime(ftyp)
			method = llvm.ConstInsertValue(method, mtyp, []uint32{2})
		}
		// typ (function type, with receiver)
		typ := tm.ToRuntime(ftyp)
		method = llvm.ConstInsertValue(method, typ, []uint32{3})

		// tfn (standard method/function pointer for plain method calls)
		tfn := llvm.ConstPtrToInt(mfunc.LLVMValue(), tm.target.IntPtrType())

		// ifn (single-word receiver function pointer for interface calls)
		ifn := tfn
		if p == nil {
			if tm.Sizeof(n) > int64(tm.target.PointerSize()) {
				if pmethodset == nil {
					pmethodset = tm.MethodSet(types.NewPointer(n))
				}
				pmfunc := tm.methodResolver.ResolveMethod(pmethodset.Lookup(sel.Obj().Pkg(), mname))
				ifn = llvm.ConstPtrToInt(pmfunc.LLVMValue(), tm.target.IntPtrType())
			} else if _, ok := n.Underlying().(*types.Pointer); !ok {
				// Create a wrapper function that takes an *int8,
				// and coerces to the receiver type.
				ifn = tm.interfaceFuncWrapper(mfunc.LLVMValue())
				ifn = llvm.ConstPtrToInt(ifn, tm.target.IntPtrType())
			}
		}

		method = llvm.ConstInsertValue(method, ifn, []uint32{4})
		method = llvm.ConstInsertValue(method, tfn, []uint32{5})
		methods[i] = method
	}
	methodsSliceType := tm.runtime.uncommonType.llvm.StructElementTypes()[2]
	methodsSlice := tm.makeSlice(methods, methodsSliceType)
	uncommonTypeInit = llvm.ConstInsertValue(uncommonTypeInit, methodsSlice, []uint32{2})
	return uncommonTypeInit
}

func (tm *TypeMap) nameRuntimeType(n *types.Named) (global, ptr llvm.Value) {
	name := typeString(n)
	path := "runtime"
	if pkg := n.Obj().Pkg(); pkg != nil {
		path = pkg.Path()
	}
	if path != tm.pkgpath {
		// We're not compiling the package from whence the type came,
		// so we'll just create a pointer to it here.
		global := llvm.AddGlobal(tm.module, tm.runtime.rtype.llvm, typeSymbol(name))
		global.SetInitializer(llvm.ConstNull(tm.runtime.rtype.llvm))
		global.SetLinkage(llvm.CommonLinkage)
		return global, global
	}

	// If the underlying type is Basic, then we always create
	// a new global. Otherwise, we clone the value returned
	// from toRuntime in case it is cached and reused.
	underlying := n.Underlying()
	if basic, ok := underlying.(*types.Basic); ok {
		global, ptr = tm.basicRuntimeType(basic, true)
		global.SetName(typeSymbol(name))
	} else {
		global, ptr = tm.toRuntime(underlying)
		clone := llvm.AddGlobal(tm.module, global.Type().ElementType(), typeSymbol(name))
		clone.SetInitializer(global.Initializer())
		global = clone
		ptr = llvm.ConstBitCast(global, llvm.PointerType(tm.runtime.rtype.llvm, 0))
	}
	global.SetLinkage(llvm.ExternalLinkage)

	// Locate the rtype.
	underlyingRuntimeType := global.Initializer()
	rtype := underlyingRuntimeType
	if rtype.Type() != tm.runtime.rtype.llvm {
		rtype = llvm.ConstExtractValue(rtype, []uint32{0})
	}

	// Insert the uncommon type.
	uncommonTypeInit := tm.uncommonType(n, nil)
	uncommonType := llvm.AddGlobal(tm.module, uncommonTypeInit.Type(), "")
	uncommonType.SetInitializer(uncommonTypeInit)
	rtype = llvm.ConstInsertValue(rtype, uncommonType, []uint32{9})

	// Replace the rtype's string representation with the one from
	// uncommonType. XXX should we have the package name prepended? Probably.
	namePtr := llvm.ConstExtractValue(uncommonTypeInit, []uint32{0})
	rtype = llvm.ConstInsertValue(rtype, namePtr, []uint32{8})

	// Update the global's initialiser. Note that we take a copy
	// of the underlying type; we're not updating a shared type.
	if underlyingRuntimeType.Type() != tm.runtime.rtype.llvm {
		underlyingRuntimeType = llvm.ConstInsertValue(underlyingRuntimeType, rtype, []uint32{0})
	} else {
		underlyingRuntimeType = rtype
	}
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

func (tm *TypeMap) makeSlice(values []llvm.Value, slicetyp llvm.Type) llvm.Value {
	ptrtyp := slicetyp.StructElementTypes()[0]
	var globalptr llvm.Value
	if len(values) > 0 {
		array := llvm.ConstArray(ptrtyp.ElementType(), values)
		globalptr = llvm.AddGlobal(tm.module, array.Type(), "")
		globalptr.SetInitializer(array)
		globalptr = llvm.ConstBitCast(globalptr, ptrtyp)
	} else {
		globalptr = llvm.ConstNull(ptrtyp)
	}
	len_ := llvm.ConstInt(tm.inttype, uint64(len(values)), false)
	slice := llvm.ConstNull(slicetyp)
	slice = llvm.ConstInsertValue(slice, globalptr, []uint32{0})
	slice = llvm.ConstInsertValue(slice, len_, []uint32{1})
	slice = llvm.ConstInsertValue(slice, len_, []uint32{2})
	return slice
}

func isGlobalObject(obj types.Object) bool {
	pkg := obj.Pkg()
	return pkg == nil || obj.Parent() == pkg.Scope()
}

func (tm *TypeMap) interfaceFuncWrapper(f llvm.Value) llvm.Value {
	ftyp := f.Type().ElementType()
	paramTypes := ftyp.ParamTypes()
	recvType := paramTypes[0]
	paramTypes[0] = llvm.PointerType(llvm.Int8Type(), 0)
	newf := llvm.AddFunction(f.GlobalParent(), f.Name()+".ifn", llvm.FunctionType(
		ftyp.ReturnType(),
		paramTypes,
		ftyp.IsFunctionVarArg(),
	))

	b := llvm.GlobalContext().NewBuilder()
	defer b.Dispose()
	entry := llvm.AddBasicBlock(newf, "entry")
	b.SetInsertPointAtEnd(entry)
	args := make([]llvm.Value, len(paramTypes))
	for i := range paramTypes {
		args[i] = newf.Param(i)
	}

	recvBits := int(tm.target.TypeSizeInBits(recvType))
	if recvBits > 0 {
		args[0] = b.CreatePtrToInt(args[0], tm.target.IntPtrType(), "")
		if args[0].Type().IntTypeWidth() > recvBits {
			args[0] = b.CreateTrunc(args[0], llvm.IntType(recvBits), "")
		}
		args[0] = coerce(b, args[0], recvType)
	} else {
		args[0] = llvm.ConstNull(recvType)
	}

	result := b.CreateCall(f, args, "")
	if result.Type().TypeKind() == llvm.VoidTypeKind {
		b.CreateRetVoid()
	} else {
		b.CreateRet(result)
	}
	return newf
}
