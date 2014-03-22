// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"bytes"
	"fmt"
	"strconv"

	"code.google.com/p/go.tools/go/ssa"
	"code.google.com/p/go.tools/go/ssa/ssautil"
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
	ctx     llvm.Context
	target     llvm.TargetData
	inttype    llvm.Type
	stringType llvm.Type

	// ptrstandin is a type used to represent the base of a
	// recursive pointer. See llgo/builder.go for how it is used
	// in CreateStore and CreateLoad.
	ptrstandin llvm.Type

	types typeutil.Map
}

type typeDescInfo struct {
	global        llvm.Value
	commonTypePtr llvm.Value
}

type TypeMap struct {
	*llvmTypeMap
	mc *manglerContext

	module         llvm.Module
	pkgpath        string
	types          typeutil.Map
	runtime        *runtimeInterface
	methodResolver MethodResolver
	alg            *algorithmMap
	types.MethodSetCache

	commonTypeType, uncommonTypeType, ptrTypeType, funcTypeType, arrayTypeType, sliceTypeType, mapTypeType, chanTypeType, interfaceTypeType llvm.Type

	imethodType llvm.Type

	typeSliceType, imethodSliceType llvm.Type

	hashFnType, equalFnType llvm.Type

	hashFnEmptyInterface, hashFnInterface, hashFnFloat, hashFnComplex, hashFnString, hashFnIdentity llvm.Value
	equalFnEmptyInterface, equalFnInterface, equalFnFloat, equalFnComplex, equalFnString, equalFnIdentity llvm.Value
}

func NewLLVMTypeMap(ctx llvm.Context, target llvm.TargetData) *llvmTypeMap {
	// spec says int is either 32-bit or 64-bit.
	// ABI currently requires sizeof(int) == sizeof(uint) == sizeof(uintptr).
	inttype := ctx.IntType(8 * target.PointerSize())

	i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
	elements := []llvm.Type{i8ptr, inttype}
	stringType := llvm.StructType(elements, false)

	return &llvmTypeMap{
		ctx:        ctx,
		StdSizes: &types.StdSizes{
			WordSize: int64(target.PointerSize()),
			MaxAlign: 8,
		},
		target:     target,
		inttype:    inttype,
		stringType: stringType,
	}
}

func NewTypeMap(pkgpath string, llvmtm *llvmTypeMap, module llvm.Module, r *runtimeInterface, mr MethodResolver, mc *manglerContext) *TypeMap {
	tm := &TypeMap{
		llvmTypeMap: llvmtm,
		mc:          mc,
		module:      module,
		pkgpath:     pkgpath,
		runtime:     r,
		methodResolver: mr,
		alg:            newAlgorithmMap(module, r, llvmtm.target),
	}

	uintptrType := tm.inttype
	voidPtrType := llvm.PointerType(tm.ctx.Int8Type(), 0)
	boolType := llvm.Int8Type()
	stringPtrType := llvm.PointerType(tm.stringType, 0)

	// Create a unique type to represent recursive pointers.
	tm.ptrstandin = llvm.GlobalContext().StructCreateNamed("")

	// Create runtime algorithm function types.
	params := []llvm.Type{voidPtrType, uintptrType}
	tm.hashFnType = llvm.FunctionType(uintptrType, params, false)
	params = []llvm.Type{voidPtrType, voidPtrType, uintptrType}
	tm.equalFnType = llvm.FunctionType(boolType, params, false)

	tm.hashFnEmptyInterface = llvm.AddFunction(tm.module, "__go_type_hash_empty_interface", tm.hashFnType)
	tm.hashFnInterface = llvm.AddFunction(tm.module, "__go_type_hash_interface", tm.hashFnType)
	tm.hashFnFloat = llvm.AddFunction(tm.module, "__go_type_hash_float", tm.hashFnType)
	tm.hashFnComplex = llvm.AddFunction(tm.module, "__go_type_hash_complex", tm.hashFnType)
	tm.hashFnString = llvm.AddFunction(tm.module, "__go_type_hash_string", tm.hashFnType)
	tm.hashFnIdentity = llvm.AddFunction(tm.module, "__go_type_hash_identity", tm.hashFnType)

	tm.equalFnEmptyInterface = llvm.AddFunction(tm.module, "__go_type_equal_empty_interface", tm.equalFnType)
	tm.equalFnInterface = llvm.AddFunction(tm.module, "__go_type_equal_interface", tm.equalFnType)
	tm.equalFnFloat = llvm.AddFunction(tm.module, "__go_type_equal_float", tm.equalFnType)
	tm.equalFnComplex = llvm.AddFunction(tm.module, "__go_type_equal_complex", tm.equalFnType)
	tm.equalFnString = llvm.AddFunction(tm.module, "__go_type_equal_string", tm.equalFnType)
	tm.equalFnIdentity = llvm.AddFunction(tm.module, "__go_type_equal_identity", tm.equalFnType)

	tm.uncommonTypeType = tm.ctx.StructCreateNamed("uncommonType")
	tm.uncommonTypeType.StructSetBody([]llvm.Type{
		stringPtrType, // name
		stringPtrType, // pkgPath
		// TODO: methods
	}, false)

	tm.commonTypeType = tm.ctx.StructCreateNamed("commonType")
	tm.commonTypeType.StructSetBody([]llvm.Type{
		tm.ctx.Int8Type(),                        // Kind
		tm.ctx.Int8Type(),                        // align
		tm.ctx.Int8Type(),                        // fieldAlign
		uintptrType,                              // size
		tm.ctx.Int32Type(),                       // hash
		llvm.PointerType(tm.hashFnType, 0),       // hashfn
		llvm.PointerType(tm.equalFnType, 0),      // equalfn
		stringPtrType,                            // string
		llvm.PointerType(tm.uncommonTypeType, 0), // uncommonType
		llvm.PointerType(tm.commonTypeType, 0),   // ptrToThis
	}, false)

	commonTypeTypePtr := llvm.PointerType(tm.commonTypeType, 0)

	tm.typeSliceType = tm.makeNamedSliceType("typeSlice", commonTypeTypePtr)

	tm.ptrTypeType = tm.ctx.StructCreateNamed("ptrType")
	tm.ptrTypeType.StructSetBody([]llvm.Type{
		tm.commonTypeType,
		commonTypeTypePtr,
	}, false)

	tm.funcTypeType = tm.ctx.StructCreateNamed("funcType")
	tm.funcTypeType.StructSetBody([]llvm.Type{
		tm.commonTypeType,
		tm.ctx.Int8Type(), // dotdotdot
		tm.typeSliceType,  // in
		tm.typeSliceType,  // out
	}, false)

	tm.arrayTypeType = tm.ctx.StructCreateNamed("arrayType")
	tm.arrayTypeType.StructSetBody([]llvm.Type{
		tm.commonTypeType,
		commonTypeTypePtr, // elem
		commonTypeTypePtr, // slice
		tm.inttype,        // len
	}, false)

	tm.sliceTypeType = tm.ctx.StructCreateNamed("sliceType")
	tm.sliceTypeType.StructSetBody([]llvm.Type{
		tm.commonTypeType,
		commonTypeTypePtr, // elem
	}, false)

	tm.mapTypeType = tm.ctx.StructCreateNamed("mapType")
	tm.mapTypeType.StructSetBody([]llvm.Type{
		tm.commonTypeType,
		commonTypeTypePtr, // key
		commonTypeTypePtr, // elem
	}, false)

	tm.chanTypeType = tm.ctx.StructCreateNamed("chanType")
	tm.chanTypeType.StructSetBody([]llvm.Type{
		tm.commonTypeType,
		commonTypeTypePtr, // elem
		tm.inttype,        // dir
	}, false)

	tm.imethodType = tm.ctx.StructCreateNamed("imethod")
	tm.imethodType.StructSetBody([]llvm.Type{
		stringPtrType,     // name
		stringPtrType,     // pkgPath
		commonTypeTypePtr, // typ
	}, false)

	tm.imethodSliceType = tm.makeNamedSliceType("imethodSlice", tm.imethodType)

	tm.interfaceTypeType = tm.ctx.StructCreateNamed("interfaceType")
	tm.interfaceTypeType.StructSetBody([]llvm.Type{
		tm.commonTypeType,
		tm.imethodSliceType,
	}, false)

	return tm
}

func (tm *llvmTypeMap) ToLLVM(t types.Type) llvm.Type {
	return tm.toLLVM(t, "")
}

func (tm *llvmTypeMap) toLLVM(t types.Type, name string) llvm.Type {
	// Signature needs to be handled specially, to preprocess
	// methods, moving the receiver to the parameter list.
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

func (tm *llvmTypeMap) makeLLVMType(t types.Type, name string) llvm.Type {
	switch t := t.(type) {
	case *types.Array:
		return tm.arrayLLVMType(t)
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
	return tm.getBackendType(t).ToLLVM(tm.ctx)
}

func (tm *llvmTypeMap) arrayLLVMType(a *types.Array) llvm.Type {
	return llvm.ArrayType(tm.ToLLVM(a.Elem()), int(a.Len()))
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

///////////////////////////////////////////////////////////////////////////////

func (tm *TypeMap) ToRuntime(t types.Type) llvm.Value {
	return tm.getTypeDescriptorPointer(t)
}

type localNamedTypeInfo struct {
	functionName string
	scopeNum     int
}

type namedTypeInfo struct {
	pkgpath string
	name    string
	localNamedTypeInfo
}

type manglerContext struct {
	ti map[*types.Named]localNamedTypeInfo
}

func (ctx *manglerContext) init(prog *ssa.Program) {
	for f, _ := range ssautil.AllFunctions(prog) {
		scopeNum := 0
		var addNamedTypesToMap func(*types.Scope)
		addNamedTypesToMap = func(scope *types.Scope) {
			hasNamedTypes := false
			for _, n := range scope.Names() {
				if tn, ok := scope.Lookup(n).(*types.TypeName); ok {
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
		if f.Synthetic == "" {
			addNamedTypesToMap(f.Object().(*types.Func).Scope())
		}
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
		if pkg := obj.Pkg(); pkg != nil {
			nti.pkgpath = obj.Pkg().Path()
		}
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
		switch t.Dir() {
		case types.SendOnly:
			b.WriteRune('s')
		case types.RecvOnly:
			b.WriteRune('r')
		case types.SendRecv:
			b.WriteString("sr")
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
			if t.Variadic() {
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

	case *types.Struct:
		b.WriteRune('S')
		for i := 0; i != t.NumFields(); i++ {
			f := t.Field(i)
			if f.Anonymous() {
				b.WriteString("0_")
			} else {
				b.WriteString(strconv.Itoa(len(f.Name())))
				b.WriteRune('_')
				b.WriteString(f.Name())
			}
			ctx.mangleType(f.Type(), b)
			// TODO: tags are mangled here
		}
		b.WriteRune('e')

	default:
		panic(fmt.Sprintf("unhandled type: %#v", t))
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
	case *types.Struct:
		return tm.commonTypeType // FIXME
	case *types.Interface:
		return tm.interfaceTypeType
	default:
		panic(fmt.Sprintf("unhandled type: %#v", t))
	}
}

func (tm *TypeMap) getNamedTypeLinkage(nt *types.Named) (linkage llvm.Linkage, emit bool) {
	if pkg := nt.Obj().Pkg(); pkg != nil {
		linkage = llvm.ExternalLinkage
		emit = pkg.Path() == tm.pkgpath
	} else {
		linkage = llvm.LinkOnceODRLinkage
		emit = true
	}

	return
}

func (tm *TypeMap) getTypeDescLinkage(t types.Type) (linkage llvm.Linkage, emit bool) {
	switch t := t.(type) {
	case *types.Named:
		linkage, emit = tm.getNamedTypeLinkage(t)

	case *types.Pointer:
		elem := t.Elem()
		if nt, ok := elem.(*types.Named); ok {
			// Thanks to the ptrToThis member, pointers to named types appear
			// in exactly the same objects as the named types themselves, so
			// we can give them the same linkage.
			linkage, emit = tm.getNamedTypeLinkage(nt)
			return
		}
		linkage = llvm.LinkOnceODRLinkage
		emit = true

	default:
		linkage = llvm.LinkOnceODRLinkage
		emit = true
	}

	return
}

func (tm *TypeMap) finalize() {
	for changed := true; changed; {
		changed = false
		tm.types.Iterate(func(key types.Type, value interface{}) {
			tdi := value.(*typeDescInfo)
			if tdi.global.Initializer().C == nil {
				linkage, emit := tm.getTypeDescLinkage(key)
				tdi.global.SetLinkage(linkage)
				if emit {
					changed = true
					tdi.global.SetInitializer(tm.makeTypeDescInitializer(key))
				}
			}
		})
	}
}

func (tm *TypeMap) makeTypeDescInitializer(t types.Type) llvm.Value {
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
	case *types.Struct:
		return tm.makeCommonType(t) // FIXME
	case *types.Interface:
		return tm.makeInterfaceType(t, u)
	default:
		panic(fmt.Sprintf("unhandled type: %#v", t))
	}
}

func (tm *TypeMap) getAlgorithmFunctions(t types.Type) (hash llvm.Value, equal llvm.Value) {
	switch t := t.Underlying().(type) {
	case *types.Interface:
		if t.NumMethods() == 0 {
			hash = tm.hashFnEmptyInterface
			equal = tm.equalFnEmptyInterface
		} else {
			hash = tm.hashFnInterface
			equal = tm.equalFnInterface
		}
	case *types.Basic:
		switch t.Kind() {
		case types.Float32, types.Float64:
			hash = tm.hashFnFloat
			equal = tm.equalFnFloat
		case types.Complex64, types.Complex128:
			hash = tm.hashFnComplex
			equal = tm.equalFnComplex
		case types.String:
			hash = tm.hashFnString
			equal = tm.equalFnString
		default:
			hash = tm.hashFnIdentity
			equal = tm.equalFnIdentity
		}
	default: // FIXME
		hash = tm.hashFnIdentity
		equal = tm.equalFnIdentity
	}

	return
}

func (tm *TypeMap) getTypeDescriptorPointer(t types.Type) llvm.Value {
	if tdi, ok := tm.types.At(t).(*typeDescInfo); ok {
		return tdi.commonTypePtr
	}

	var b bytes.Buffer
	tm.mc.mangleTypeDescriptorName(t, &b)

	global := llvm.AddGlobal(tm.module, tm.getTypeDescType(t), b.String())
	ptr := llvm.ConstBitCast(global, llvm.PointerType(tm.commonTypeType, 0))
	tm.types.Set(t, &typeDescInfo{global: global, commonTypePtr: ptr})

	return ptr
}

const (
	// From gofrontend/types.h
	gccgoRuntimeTypeKindBOOL           = 1
	gccgoRuntimeTypeKindINT            = 2
	gccgoRuntimeTypeKindINT8           = 3
	gccgoRuntimeTypeKindINT16          = 4
	gccgoRuntimeTypeKindINT32          = 5
	gccgoRuntimeTypeKindINT64          = 6
	gccgoRuntimeTypeKindUINT           = 7
	gccgoRuntimeTypeKindUINT8          = 8
	gccgoRuntimeTypeKindUINT16         = 9
	gccgoRuntimeTypeKindUINT32         = 10
	gccgoRuntimeTypeKindUINT64         = 11
	gccgoRuntimeTypeKindUINTPTR        = 12
	gccgoRuntimeTypeKindFLOAT32        = 13
	gccgoRuntimeTypeKindFLOAT64        = 14
	gccgoRuntimeTypeKindCOMPLEX64      = 15
	gccgoRuntimeTypeKindCOMPLEX128     = 16
	gccgoRuntimeTypeKindARRAY          = 17
	gccgoRuntimeTypeKindCHAN           = 18
	gccgoRuntimeTypeKindFUNC           = 19
	gccgoRuntimeTypeKindINTERFACE      = 20
	gccgoRuntimeTypeKindMAP            = 21
	gccgoRuntimeTypeKindPTR            = 22
	gccgoRuntimeTypeKindSLICE          = 23
	gccgoRuntimeTypeKindSTRING         = 24
	gccgoRuntimeTypeKindSTRUCT         = 25
	gccgoRuntimeTypeKindUNSAFE_POINTER = 26
	gccgoRuntimeTypeKindNO_POINTERS    = (1 << 7)
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
	hash, equal := tm.getAlgorithmFunctions(t)
	vals[5] = hash
	vals[6] = equal
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
	if f.Variadic() {
		variadic = 1
	}
	vals[1] = llvm.ConstInt(llvm.Int8Type(), uint64(variadic), false)
	// in
	vals[2] = tm.rtypeSlice(f.Params())
	// out
	vals[3] = tm.rtypeSlice(f.Results())

	return llvm.ConstNamedStruct(tm.funcTypeType, vals[:])
}

func (tm *TypeMap) makeInterfaceType(t types.Type, i *types.Interface) llvm.Value {
	var vals [2]llvm.Value
	vals[0] = tm.makeCommonType(t)

	methodset := tm.MethodSet(i)
	imethods := make([]llvm.Value, methodset.Len())
	for index := 0; index < methodset.Len(); index++ {
		method := methodset.At(index).Obj()
		var imvals [3]llvm.Value
		imvals[0] = tm.globalStringPtr(method.Name())
		imvals[1] = llvm.ConstPointerNull(llvm.PointerType(tm.stringType, 0))
		imvals[2] = tm.getTypeDescriptorPointer(method.Type())

		imethods[index] = llvm.ConstNamedStruct(tm.imethodType, imvals[:])
	}
	vals[1] = tm.makeSlice(imethods, tm.imethodSliceType)

	return llvm.ConstNamedStruct(tm.interfaceTypeType, vals[:])
}

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
	var dir int
	switch c.Dir() {
	case types.RecvOnly:
		dir = 1
	case types.SendOnly:
		dir = 2
	case types.SendRecv:
		dir = 3
	}
	vals[2] = llvm.ConstInt(tm.inttype, uint64(dir), false)

	return llvm.ConstNamedStruct(tm.chanTypeType, vals[:])
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

/*
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
*/

// globalStringPtr returns a *string with the specified value.
func (tm *TypeMap) globalStringPtr(value string) llvm.Value {
	strval := llvm.ConstString(value, false)
	strglobal := llvm.AddGlobal(tm.module, strval.Type(), "")
	strglobal.SetLinkage(llvm.InternalLinkage)
	strglobal.SetInitializer(strval)
	strglobal = llvm.ConstBitCast(strglobal, llvm.PointerType(llvm.Int8Type(), 0))
	strlen := llvm.ConstInt(tm.inttype, uint64(len(value)), false)
	str := llvm.ConstStruct([]llvm.Value{strglobal, strlen}, false)
	g := llvm.AddGlobal(tm.module, str.Type(), "")
	g.SetLinkage(llvm.InternalLinkage)
	g.SetInitializer(str)
	return g
}

func (tm *TypeMap) makeNamedSliceType(tname string, elemtyp llvm.Type) llvm.Type {
	t := tm.ctx.StructCreateNamed(tname)
	t.StructSetBody([]llvm.Type{
		llvm.PointerType(elemtyp, 0),
		tm.inttype,
		tm.inttype,
	}, false)
	return t
}

func (tm *TypeMap) makeSlice(values []llvm.Value, slicetyp llvm.Type) llvm.Value {
	ptrtyp := slicetyp.StructElementTypes()[0]
	var globalptr llvm.Value
	if len(values) > 0 {
		array := llvm.ConstArray(ptrtyp.ElementType(), values)
		globalptr = llvm.AddGlobal(tm.module, array.Type(), "")
		globalptr.SetLinkage(llvm.InternalLinkage)
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
