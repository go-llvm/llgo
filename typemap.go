// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"bytes"
	"code.google.com/p/go.tools/go/types"
	"code.google.com/p/go.tools/go/types/typemap"
	"code.google.com/p/go.tools/ssa"
	"fmt"
	"github.com/axw/gollvm/llvm"
	"go/ast"
	"strconv"
)

// llvmTypeMap is provides a means of mapping from a types.Map
// to llgo's corresponding LLVM type representation.
type llvmTypeMap struct {
	TypeStringer
	*types.StdSizes
	target  llvm.TargetData
	inttype llvm.Type
	stringType llvm.Type

	// ptrstandin is a type used to represent the base of a
	// recursive pointer. See llgo/builder.go for how it is used
	// in CreateStore and CreateLoad.
	ptrstandin llvm.Type

	types map[string]llvm.Type // compile-time LLVM type
}

type typeDescInfo struct {
	global llvm.Value
	commonTypePtr llvm.Value
}

type TypeMap struct {
	*llvmTypeMap
	mc *manglerContext

	ctx     llvm.Context
	module  llvm.Module
	pkgpath string
	types   typemap.M // Type -> *typeDescInfo
	runtime *runtimeInterface

	commonTypeType, uncommonTypeType, ptrTypeType, funcTypeType, arrayTypeType, sliceTypeType, mapTypeType, chanTypeType llvm.Type

	typeSliceType llvm.Type

	hashFnType, equalFnType llvm.Type
}

func NewLLVMTypeMap(ctx llvm.Context, target llvm.TargetData) *llvmTypeMap {
	// spec says int is either 32-bit or 64-bit. 
	// ABI currently requires sizeof(int) == sizeof(uint) == sizeof(uintptr).
	inttype := ctx.IntType(target.PointerSize())

	i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
	elements := []llvm.Type{i8ptr, inttype}
	stringType := llvm.StructType(elements, false)

	return &llvmTypeMap{
		StdSizes: &types.StdSizes{
			WordSize: int64(target.PointerSize()),
			MaxAlign: 8,
		},
		target:  target,
		types:   make(map[string]llvm.Type),
		inttype: inttype,
		stringType: stringType,
	}
}

func NewTypeMap(pkgpath string, llvmtm *llvmTypeMap, module llvm.Module, r *runtimeInterface) *TypeMap {
	tm := &TypeMap{
		llvmTypeMap: llvmtm,
		module:      module,
		pkgpath:     pkgpath,
		runtime:     r,
	}

	ctx := module.Context()

	uintptrType := tm.inttype
	voidPtrType := llvm.PointerType(ctx.Int8Type(), 0)
	boolType := llvm.Int1Type()
	stringPtrType := llvm.PointerType(tm.stringType, 0)

	// Create runtime algorithm function types.
	params := []llvm.Type{voidPtrType, uintptrType}
	tm.hashFnType = llvm.FunctionType(uintptrType, params, false)
	params = []llvm.Type{voidPtrType, voidPtrType, uintptrType}
	tm.equalFnType = llvm.FunctionType(boolType, params, false)

	tm.uncommonTypeType = ctx.StructCreateNamed("uncommonType")
	tm.uncommonTypeType.StructSetBody([]llvm.Type{
		stringPtrType, // name
		stringPtrType, // pkgPath
		// TODO: methods
	}, false)

	tm.commonTypeType = ctx.StructCreateNamed("commonType")
	tm.commonTypeType.StructSetBody([]llvm.Type{
		ctx.Int8Type(), // Kind
		ctx.Int8Type(), // align
		ctx.Int8Type(), // fieldAlign
		uintptrType,    // size
		ctx.Int32Type(),// hash
		llvm.PointerType(tm.hashFnType, 0),// hashfn
		llvm.PointerType(tm.equalFnType, 0),// equalfn
		stringPtrType, // string
		llvm.PointerType(tm.uncommonTypeType, 0), // uncommonType
		llvm.PointerType(tm.commonTypeType, 0), // ptrToThis
	}, false)

	commonTypeTypePtr := llvm.PointerType(tm.commonTypeType, 0)

	tm.typeSliceType = ctx.StructCreateNamed("typeSliceType")
	tm.typeSliceType.StructSetBody([]llvm.Type{
		llvm.PointerType(commonTypeTypePtr, 0),
		tm.inttype,
		tm.inttype,
	}, false)

	tm.ptrTypeType = ctx.StructCreateNamed("ptrType")
	tm.ptrTypeType.StructSetBody([]llvm.Type{
		tm.commonTypeType,
		commonTypeTypePtr,
	}, false)

	tm.funcTypeType = ctx.StructCreateNamed("funcType")
	tm.funcTypeType.StructSetBody([]llvm.Type{
		tm.commonTypeType,
		ctx.Int8Type(), // dotdotdot
		tm.typeSliceType, // in
		tm.typeSliceType, // out
	}, false)

	tm.arrayTypeType = ctx.StructCreateNamed("arrayType")
	tm.arrayTypeType.StructSetBody([]llvm.Type{
		tm.commonTypeType,
		commonTypeTypePtr, // elem
		commonTypeTypePtr, // slice
		tm.inttype, // len
	}, false)

	tm.sliceTypeType = ctx.StructCreateNamed("sliceType")
	tm.sliceTypeType.StructSetBody([]llvm.Type{
		tm.commonTypeType,
		commonTypeTypePtr, // elem
	}, false)

	tm.mapTypeType = ctx.StructCreateNamed("mapType")
	tm.mapTypeType.StructSetBody([]llvm.Type{
		tm.commonTypeType,
		commonTypeTypePtr, // key
		commonTypeTypePtr, // elem
	}, false)

	tm.chanTypeType = ctx.StructCreateNamed("chanType")
	tm.chanTypeType.StructSetBody([]llvm.Type{
		tm.commonTypeType,
		commonTypeTypePtr, // elem
		tm.inttype, // dir
	}, false)

	return tm
}

func (tm *llvmTypeMap) ToLLVM(t types.Type) llvm.Type {
	tstr := tm.TypeKey(t)
	lt, ok := tm.types[tstr]
	if !ok {
		lt = tm.makeLLVMType(tstr, t)
		if lt.IsNil() {
			panic(fmt.Sprint("Failed to create LLVM type for: ", tstr))
		}
	}
	return lt
}

func (tm *llvmTypeMap) makeLLVMType(tstr string, t types.Type) llvm.Type {
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
	case types.Uint, types.Int, types.Uintptr:
		return tm.inttype
	case types.Int64, types.Uint64:
		return llvm.Int64Type()
	case types.Float32:
		return llvm.FloatType()
	case types.Float64:
		return llvm.DoubleType()
	case types.UnsafePointer:
		return llvm.PointerType(llvm.Int8Type(), 0)
	case types.Complex64:
		f32 := llvm.FloatType()
		elements := []llvm.Type{f32, f32}
		return llvm.StructType(elements, false)
	case types.Complex128:
		f64 := llvm.DoubleType()
		elements := []llvm.Type{f64, f64}
		return llvm.StructType(elements, false)
	case types.String:
		return tm.stringType
	}
	panic(fmt.Sprint("unhandled kind: ", b.Kind))
}

func (tm *llvmTypeMap) arrayLLVMType(a *types.Array) llvm.Type {
	return llvm.ArrayType(tm.ToLLVM(a.Elem()), int(a.Len()))
}

func (tm *llvmTypeMap) sliceLLVMType(tstr string, s *types.Slice) llvm.Type {
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

func (tm *llvmTypeMap) structLLVMType(tstr string, s *types.Struct) llvm.Type {
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

func (tm *llvmTypeMap) pointerLLVMType(p *types.Pointer) llvm.Type {
	elem := p.Elem()
	if p, ok := elem.Underlying().(*types.Pointer); ok && p.Elem() == elem {
		// Recursive pointers must be handled specially, as
		// LLVM does not permit recursive types except via
		// named structs.
		if tm.ptrstandin.IsNil() {
			ctx := llvm.GlobalContext()
			unique := ctx.StructCreateNamed("")
			tm.ptrstandin = unique
		}
		return llvm.PointerType(tm.ptrstandin, 0)
	}
	return llvm.PointerType(tm.ToLLVM(p.Elem()), 0)
}

func (tm *llvmTypeMap) funcLLVMType(tstr string, f *types.Signature) llvm.Type {
	if typ, ok := tm.types[tstr]; ok {
		return typ
	}

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
		f := types.NewSignature(nil, nil, params, f.Results(), f.IsVariadic())
		return tm.ToLLVM(f)
	}

	typ := llvm.GlobalContext().StructCreateNamed("")
	tm.types[tstr] = typ

	params := f.Params()
	nparams := int(params.Len())
	for i := 0; i < nparams; i++ {
		llvmtyp := tm.ToLLVM(params.At(i).Type())
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
	return typ
}

func (tm *llvmTypeMap) interfaceLLVMType(tstr string, i *types.Interface) llvm.Type {
	if typ, ok := tm.types[tstr]; ok {
		return typ
	}
	// interface{} is represented as {type, value},
	// and non-empty interfaces are represented as {itab, value}.
	i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
	rtypeType := i8ptr
	valueType := i8ptr
	typ := llvm.GlobalContext().StructCreateNamed("")
	typ.StructSetBody([]llvm.Type{rtypeType, valueType}, false)
	tm.types[tstr] = typ
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
	// TODO propagate name through to underlying type.
	return tm.ToLLVM(n.Underlying())
}

///////////////////////////////////////////////////////////////////////////////

func (tm *TypeMap) ToRuntime(t types.Type) llvm.Value {
	return tm.getTypeDescriptorPointer(t)
}

type localNamedTypeInfo struct {
	functionName string
	scopeNum int
}

type namedTypeInfo struct {
	pkgpath string
	name string
	localNamedTypeInfo
}

type manglerContext struct {
	ti map[*types.Named]localNamedTypeInfo
}

func (ctx *manglerContext) init(prog *ssa.Program) {
	for f, _ := range ssa.AllFunctions(prog) {
		scopeNum := 0
		var addNamedTypesToMap func(*types.Scope)
		addNamedTypesToMap = func(scope *types.Scope) {
			hasNamedTypes := false
			for _, n := range scope.Names() {
				tn := scope.Lookup(n).(*types.TypeName)
				if tn != nil {
					hasNamedTypes = true
					ctx.ti[tn.Type().(*types.Named)] = localNamedTypeInfo{f.Name(), scopeNum}
				}
			}
			if hasNamedTypes {
				scopeNum++
			}
			for i := 0; i != scope.NumChildren(); i++ {
				addNamedTypesToMap(scope.Child(i))
			}
		}
		addNamedTypesToMap(f.Object().(*types.Func).Scope())
	}
}

func (ctx *manglerContext) getNamedTypeInfo(t types.Type) (nti namedTypeInfo) {
	switch t := t.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Byte:
			nti.name = "uint8"
		case types.Rune:
			nti.name = "int32"
		case types.UnsafePointer:
			nti.pkgpath = "unsafe"
			nti.name = "Pointer"
		default:
			nti.name = t.Name()
		}

	case *types.Named:
		obj := t.Obj()
		nti.pkgpath = obj.Pkg().Path()
		nti.name = obj.Name()
		nti.localNamedTypeInfo = ctx.ti[t]

	default:
		panic("not a named type")
	}

	return
}

func (ctx *manglerContext) mangleType(t types.Type, b *bytes.Buffer) {
	switch t := t.(type) {
	case *types.Basic, *types.Named:
		var nb bytes.Buffer
		ti := ctx.getNamedTypeInfo(t)
		if ti.pkgpath != "" {
			nb.WriteString(ti.pkgpath)
			nb.WriteRune('.')
		}
		if ti.functionName != "" {
			nb.WriteString(ti.functionName)
			nb.WriteRune('$')
			if ti.scopeNum != 0 {
				nb.WriteString(strconv.Itoa(ti.scopeNum))
				nb.WriteRune('$')
			}
		}
		nb.WriteString(ti.name)

		b.WriteRune('N')
		b.WriteString(strconv.Itoa(nb.Len()))
		b.WriteRune('_')
		b.WriteString(nb.String())

	case *types.Pointer:
		b.WriteRune('p')
		ctx.mangleType(t.Elem(), b)

	case *types.Map:
		b.WriteRune('M')
		ctx.mangleType(t.Key(), b)
		b.WriteString("__")
		ctx.mangleType(t.Elem(), b)

	case *types.Chan:
		b.WriteRune('C')
		ctx.mangleType(t.Elem(), b)
		if t.Dir() & ast.SEND != 0 {
			b.WriteRune('s')
		}
		if t.Dir() & ast.RECV != 0 {
			b.WriteRune('r')
		}
		b.WriteRune('e')

	case *types.Signature:
		b.WriteRune('F')
		if recv := t.Recv(); recv != nil {
			b.WriteRune('m')
			ctx.mangleType(recv.Type(), b)
		}

		if p := t.Params(); p.Len() != 0 {
			b.WriteRune('p')
			for i := 0; i != p.Len(); i++ {
				ctx.mangleType(p.At(i).Type(), b)
			}
			if t.IsVariadic() {
				b.WriteRune('V')
			}
			b.WriteRune('e')
		}

		if r := t.Results(); r.Len() != 0 {
			b.WriteRune('r')
			for i := 0; i != r.Len(); i++ {
				ctx.mangleType(r.At(i).Type(), b)
			}
			b.WriteRune('e')
		}

	case *types.Array:
		b.WriteRune('A')
		ctx.mangleType(t.Elem(), b)
		b.WriteString(strconv.FormatInt(t.Len(), 10))
		b.WriteRune('e')

	case *types.Slice:
		b.WriteRune('A')
		ctx.mangleType(t.Elem(), b)
		b.WriteRune('e')

	default:
		panic("type not handled yet")
	}
}

func (ctx *manglerContext) mangleTypeDescriptorName(t types.Type, b *bytes.Buffer) {
	switch t := t.(type) {
	case *types.Basic, *types.Named:
		b.WriteString("__go_tdn_")
		ti := ctx.getNamedTypeInfo(t)
		if ti.pkgpath != "" {
			b.WriteString(ti.pkgpath)
			b.WriteRune('.')
		}
		if ti.functionName != "" {
			b.WriteString(ti.functionName)
			b.WriteRune('.')
			if ti.scopeNum != 0 {
				b.WriteString(strconv.Itoa(ti.scopeNum))
				b.WriteRune('.')
			}
		}
		b.WriteString(ti.name)

	default:
		b.WriteString("__go_td_")
		ctx.mangleType(t, b)
	}
}

func (tm *TypeMap) getTypeDescType(t types.Type) llvm.Type {
	switch t.Underlying().(type) {
	case *types.Basic:
		return tm.commonTypeType
	case *types.Pointer:
		return tm.ptrTypeType
	case *types.Signature:
		return tm.funcTypeType
	case *types.Array:
		return tm.arrayTypeType
	case *types.Slice:
		return tm.sliceTypeType
	case *types.Map:
		return tm.mapTypeType
	case *types.Chan:
		return tm.chanTypeType
	default:
		panic(fmt.Sprintf("unhandled type: %#v", t))
	}
}

func (tm *TypeMap) makeTypeDescInitializer(tstr string, t types.Type) llvm.Value {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return tm.makeBasicType(t, u)
	case *types.Pointer:
		return tm.makePointerType(t, u)
	case *types.Signature:
		return tm.makeFuncType(t, u)
	case *types.Array:
		return tm.makeArrayType(t, u)
	case *types.Slice:
		return tm.makeSliceType(t, u)
	case *types.Map:
		return tm.makeMapType(t, u)
	case *types.Chan:
		return tm.makeChanType(t, u)
	default:
		panic(fmt.Sprintf("unhandled type: %#v", t))
	}
}

func (tm *TypeMap) makeAlgorithmTable(t types.Type) llvm.Value {
	elems := []llvm.Value{}
	return llvm.ConstStruct(elems, false)
}

func (tm *TypeMap) getTypeDescriptorPointer(t types.Type) llvm.Value {
	if tdi, ok := tm.types.At(t).(*typeDescInfo); ok {
		return tdi.commonTypePtr
	}

	var b bytes.Buffer
	tm.mc.mangleType(t, &b)

	global := llvm.AddGlobal(tm.module, tm.getTypeDescType(t), b.String())
	ptr := llvm.ConstBitCast(global, llvm.PointerType(tm.commonTypeType, 0))
	tm.types.Set(t, &typeDescInfo{global: global, commonTypePtr: ptr})

	return ptr
}

func (tm *TypeMap) makeRuntimeTypeGlobal(v llvm.Value) (global, ptr llvm.Value) {
	global = llvm.AddGlobal(tm.module, v.Type(), "")
	global.SetInitializer(v)
	ptr = llvm.ConstBitCast(global, llvm.PointerType(tm.runtime.rtype.llvm, 0))
	return global, ptr
}

const (
	// From gofrontend/types.h
	gccgoRuntimeTypeKindBOOL = 1
	gccgoRuntimeTypeKindINT = 2
	gccgoRuntimeTypeKindINT8 = 3
	gccgoRuntimeTypeKindINT16 = 4
	gccgoRuntimeTypeKindINT32 = 5
	gccgoRuntimeTypeKindINT64 = 6
	gccgoRuntimeTypeKindUINT = 7
	gccgoRuntimeTypeKindUINT8 = 8
	gccgoRuntimeTypeKindUINT16 = 9
	gccgoRuntimeTypeKindUINT32 = 10
	gccgoRuntimeTypeKindUINT64 = 11
	gccgoRuntimeTypeKindUINTPTR = 12
	gccgoRuntimeTypeKindFLOAT32 = 13
	gccgoRuntimeTypeKindFLOAT64 = 14
	gccgoRuntimeTypeKindCOMPLEX64 = 15
	gccgoRuntimeTypeKindCOMPLEX128 = 16
	gccgoRuntimeTypeKindARRAY = 17
	gccgoRuntimeTypeKindCHAN = 18
	gccgoRuntimeTypeKindFUNC = 19
	gccgoRuntimeTypeKindINTERFACE = 20
	gccgoRuntimeTypeKindMAP = 21
	gccgoRuntimeTypeKindPTR = 22
	gccgoRuntimeTypeKindSLICE = 23
	gccgoRuntimeTypeKindSTRING = 24
	gccgoRuntimeTypeKindSTRUCT = 25
	gccgoRuntimeTypeKindUNSAFE_POINTER = 26
	gccgoRuntimeTypeKindNO_POINTERS = (1 << 7)
)

func hasPointers(t types.Type) bool {
	switch t := t.(type) {
	case *types.Basic:
		return t.Kind() == types.UnsafePointer

	case *types.Signature, *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Interface:
		return true

	case *types.Struct:
		for i := 0; i != t.NumFields(); i++ {
			if hasPointers(t.Field(i).Type()) {
				return true
			}
		}
		return false

	case *types.Named:
		return hasPointers(t.Underlying())

	case *types.Array:
		return hasPointers(t.Elem())

	default:
		panic("unrecognized type")
	}
}

func runtimeTypeKind(t types.Type) (k uint8) {
	switch t := t.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Bool:
			k = gccgoRuntimeTypeKindBOOL
		case types.Int:
			k = gccgoRuntimeTypeKindINT
		case types.Int8:
			k = gccgoRuntimeTypeKindINT8
		case types.Int16:
			k = gccgoRuntimeTypeKindINT16
		case types.Int32:
			k = gccgoRuntimeTypeKindINT32
		case types.Int64:
			k = gccgoRuntimeTypeKindINT64
		case types.Uint:
			k = gccgoRuntimeTypeKindUINT
		case types.Uint8:
			k = gccgoRuntimeTypeKindUINT8
		case types.Uint16:
			k = gccgoRuntimeTypeKindUINT16
		case types.Uint32:
			k = gccgoRuntimeTypeKindUINT32
		case types.Uint64:
			k = gccgoRuntimeTypeKindUINT64
		case types.Uintptr:
			k = gccgoRuntimeTypeKindUINTPTR
		case types.Float32:
			k = gccgoRuntimeTypeKindFLOAT32
		case types.Float64:
			k = gccgoRuntimeTypeKindFLOAT64
		case types.Complex64:
			k = gccgoRuntimeTypeKindCOMPLEX64
		case types.Complex128:
			k = gccgoRuntimeTypeKindCOMPLEX128
		case types.String:
			k = gccgoRuntimeTypeKindSTRING
		case types.UnsafePointer:
			k = gccgoRuntimeTypeKindUNSAFE_POINTER
		default:
			panic("unrecognized builtin type")
		}
	case *types.Array:
		k = gccgoRuntimeTypeKindARRAY
	case *types.Slice:
		k = gccgoRuntimeTypeKindSLICE
	case *types.Struct:
		k = gccgoRuntimeTypeKindSTRUCT
	case *types.Pointer:
		k = gccgoRuntimeTypeKindPTR
	case *types.Signature:
		k = gccgoRuntimeTypeKindFUNC
	case *types.Interface:
		k = gccgoRuntimeTypeKindINTERFACE
	case *types.Map:
		k = gccgoRuntimeTypeKindMAP
	case *types.Chan:
		k = gccgoRuntimeTypeKindCHAN
	case *types.Named:
		return runtimeTypeKind(t.Underlying())
	default:
		panic("unrecognized type")
	}

	if !hasPointers(t) {
		k |= gccgoRuntimeTypeKindNO_POINTERS
	}

	return
}

func (tm *TypeMap) makeCommonType(t types.Type) llvm.Value {
	var vals [10]llvm.Value
	vals[0] = llvm.ConstInt(tm.ctx.Int8Type(), uint64(runtimeTypeKind(t)), false)
	vals[1] = llvm.ConstInt(tm.ctx.Int8Type(), uint64(tm.Alignof(t)), false)
	vals[2] = vals[1]
	vals[3] = llvm.ConstInt(tm.inttype, uint64(tm.Sizeof(t)), false)
	vals[4] = llvm.ConstInt(tm.ctx.Int32Type(), 42, false)
	vals[5] = llvm.ConstPointerNull(llvm.PointerType(tm.hashFnType, 0))
	vals[6] = llvm.ConstPointerNull(llvm.PointerType(tm.equalFnType, 0))
	vals[7] = llvm.ConstPointerNull(llvm.PointerType(tm.stringType, 0))
	vals[8] = llvm.ConstPointerNull(llvm.PointerType(tm.uncommonTypeType, 0))
	if _, ok := t.(*types.Named); ok {
		vals[9] = tm.getTypeDescriptorPointer(types.NewPointer(t))
	} else {
		vals[9] = llvm.ConstPointerNull(llvm.PointerType(tm.commonTypeType, 0))
	}

	return llvm.ConstNamedStruct(tm.commonTypeType, vals[:])
}

func (tm *TypeMap) makeBasicType(t types.Type, u *types.Basic) llvm.Value {
	return tm.makeCommonType(t)
}

func (tm *TypeMap) makeArrayType(t types.Type, a *types.Array) llvm.Value {
	var vals [4]llvm.Value
	vals[0] = tm.makeCommonType(t)
	vals[1] = tm.getTypeDescriptorPointer(a.Elem())
	vals[2] = tm.getTypeDescriptorPointer(types.NewSlice(a.Elem()))
	vals[3] = llvm.ConstInt(tm.inttype, uint64(a.Len()), false)

	return llvm.ConstNamedStruct(tm.arrayTypeType, vals[:])
}

func (tm *TypeMap) makeSliceType(t types.Type, s *types.Slice) llvm.Value {
	var vals [2]llvm.Value
	vals[0] = tm.makeCommonType(t)
	vals[1] = tm.getTypeDescriptorPointer(s.Elem())

	return llvm.ConstNamedStruct(tm.sliceTypeType, vals[:])
}

/*
func (tm *TypeMap) structRuntimeType(tstr string, s *types.Struct) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(s, reflect.Struct)
	structType := llvm.ConstNull(tm.runtime.structType.llvm)
	structType = llvm.ConstInsertValue(structType, rtype, []uint32{0})
	global, ptr = tm.makeRuntimeTypeGlobal(structType)
	tm.types.record(tstr, global, ptr)
	// TODO set fields, reset initialiser
	return global, ptr
}
*/

func (tm *TypeMap) makePointerType(t types.Type, p *types.Pointer) llvm.Value {
	var vals [2]llvm.Value
	vals[0] = tm.makeCommonType(t)
	vals[1] = tm.getTypeDescriptorPointer(p.Elem())

	return llvm.ConstNamedStruct(tm.ptrTypeType, vals[:])
}

func (tm *TypeMap) rtypeSlice(t *types.Tuple) llvm.Value {
	rtypes := make([]llvm.Value, t.Len())
	for i := range rtypes {
		rtypes[i] = tm.getTypeDescriptorPointer(t.At(i).Type())
	}
	return tm.makeSlice(rtypes, tm.typeSliceType)
}

func (tm *TypeMap) makeFuncType(t types.Type, f *types.Signature) llvm.Value {
	var vals [4]llvm.Value
	vals[0] = tm.makeCommonType(t)
	// dotdotdot
	variadic := 0
	if f.IsVariadic() {
		variadic = 1
	}
	vals[1] = llvm.ConstInt(llvm.Int8Type(), uint64(variadic), false)
	// in
	vals[2] = tm.rtypeSlice(f.Params())
	// out
	vals[3] = tm.rtypeSlice(f.Results())

	return llvm.ConstNamedStruct(tm.funcTypeType, vals[:])
}

/*
func (tm *TypeMap) interfaceRuntimeType(tstr string, i *types.Interface) (global, ptr llvm.Value) {
	rtype := tm.makeRtype(i, reflect.Interface)
	interfaceType := llvm.ConstNull(tm.runtime.interfaceType.llvm)
	global, ptr = tm.makeRuntimeTypeGlobal(interfaceType)
	tm.types.record(tstr, global, ptr)
	interfaceType = llvm.ConstInsertValue(interfaceType, rtype, []uint32{0})
	methodset := i.MethodSet()
	imethods := make([]llvm.Value, methodset.Len())
	for index := 0; index < methodset.Len(); index++ {
		method := methodset.At(index).Obj()
		imethod := llvm.ConstNull(tm.runtime.imethod.llvm)
		name := tm.globalStringPtr(method.Name())
		name = llvm.ConstBitCast(name, tm.runtime.imethod.llvm.StructElementTypes()[0])
		//pkgpath := tm.globalStringPtr(tm.functions.objectdata[method].Package.Path())
		//pkgpath = llvm.ConstBitCast(name, tm.runtime.imethod.llvm.StructElementTypes()[1])
		mtyp := tm.ToRuntime(method.Type())
		imethod = llvm.ConstInsertValue(imethod, name, []uint32{0})
		//imethod = llvm.ConstInsertValue(imethod, pkgpath, []uint32{1})
		imethod = llvm.ConstInsertValue(imethod, mtyp, []uint32{2})
		imethods[index] = imethod
	}
	imethodsSliceType := tm.runtime.interfaceType.llvm.StructElementTypes()[1]
	imethodsSlice := tm.makeSlice(imethods, imethodsSliceType)
	interfaceType = llvm.ConstInsertValue(interfaceType, imethodsSlice, []uint32{1})
	global.SetInitializer(interfaceType)
	return global, ptr
}
*/

func (tm *TypeMap) makeMapType(t types.Type, m *types.Map) llvm.Value {
	var vals [3]llvm.Value
	vals[0] = tm.makeCommonType(t)
	vals[1] = tm.getTypeDescriptorPointer(m.Key())
	vals[2] = tm.getTypeDescriptorPointer(m.Elem())

	return llvm.ConstNamedStruct(tm.mapTypeType, vals[:])
}

func (tm *TypeMap) makeChanType(t types.Type, c *types.Chan) llvm.Value {
	var vals [3]llvm.Value
	vals[0] = tm.makeCommonType(t)
	vals[1] = tm.getTypeDescriptorPointer(c.Elem())

	// From gofrontend/go/types.cc
	// These bits must match the ones in libgo/runtime/go-type.h.
	dir := 0
	if c.Dir()&ast.RECV != 0 {
		dir = 1
	}
	if c.Dir()&ast.SEND != 0 {
		dir |= 2
	}
	vals[2] = llvm.ConstInt(tm.inttype, uint64(dir), false)

	return llvm.ConstNamedStruct(tm.chanTypeType, vals[:])
}

func (tm *TypeMap) uncommonType(n *types.Named, ptr bool) llvm.Value {
	uncommonTypeInit := llvm.ConstNull(tm.runtime.uncommonType.llvm)
	namePtr := tm.globalStringPtr(n.Obj().Name())
	uncommonTypeInit = llvm.ConstInsertValue(uncommonTypeInit, namePtr, []uint32{0})

	_, path := tm.qualifiedName(n)
	pkgpathPtr := tm.globalStringPtr(path)
	uncommonTypeInit = llvm.ConstInsertValue(uncommonTypeInit, pkgpathPtr, []uint32{1})

	/*
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
				ftyp := types.NewSignature(nil, nil, ftyp.Params(), ftyp.Results(), ftyp.IsVariadic())
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
		methodsSliceType := tm.runtime.uncommonType.llvm.StructElementTypes()[2]
		methodsSlice := tm.makeSlice(methods, methodsSliceType)
		uncommonTypeInit = llvm.ConstInsertValue(uncommonTypeInit, methodsSlice, []uint32{2})
	*/
	return uncommonTypeInit
}

func (tm *TypeMap) qualifiedName(n *types.Named) (qname, path string) {
	pkg := n.Obj().Pkg()
	if pkg == nil {
		return n.Obj().Name(), ""
	}
	path = pkg.Path()
	return path + "." + n.Obj().Name(), path
}

/*
func (tm *TypeMap) nameRuntimeType(n *types.Named) (global, ptr llvm.Value) {
	qname, path := tm.qualifiedName(n)
	globalname := "__llgo.type." + qname
	if path != tm.pkgpath {
		// We're not compiling the package from whence the type came,
		// so we'll just create a pointer to it here.
		global := llvm.AddGlobal(tm.module, tm.runtime.rtype.llvm, globalname)
		global.SetInitializer(llvm.ConstNull(tm.runtime.rtype.llvm))
		global.SetLinkage(llvm.CommonLinkage)
		return global, global
	} else if !isGlobalObject(n.Obj()) {
		globalname = ""
	}

	// If the underlying type is Basic, then we always create
	// a new global. Otherwise, we clone the value returned
	// from toRuntime in case it is cached and reused.
	underlying := n.Underlying()
	if basic, ok := underlying.(*types.Basic); ok {
		global, ptr = tm.basicRuntimeType(basic, true)
	} else {
		global, ptr = tm.toRuntime(underlying)
		clone := llvm.AddGlobal(tm.module, global.Type().ElementType(), globalname)
		clone.SetInitializer(global.Initializer())
		global = clone
		ptr = llvm.ConstBitCast(global, llvm.PointerType(tm.runtime.rtype.llvm, 0))
	}

	// Locate the rtype.
	underlyingRuntimeType := global.Initializer()
	rtype := underlyingRuntimeType
	if rtype.Type() != tm.runtime.rtype.llvm {
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
	if underlyingRuntimeType.Type() != tm.runtime.rtype.llvm {
		underlyingRuntimeType = llvm.ConstInsertValue(underlyingRuntimeType, rtype, []uint32{0})
	} else {
		underlyingRuntimeType = rtype
	}
	global.SetName(globalname)
	global.SetInitializer(underlyingRuntimeType)
	return global, ptr
}
*/

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
