// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/token"
	"path"
	"strconv"

	"code.google.com/p/go.tools/go/types"

	"github.com/axw/gollvm/llvm"
)

type FuncResolver interface {
	ResolveFunc(*types.Func) *LLVMValue
}

type runtimeType struct {
	types.Type
	llvm llvm.Type
}

type runtimeFnInfo struct {
	fi *functionTypeInfo
	fn llvm.Value
}

func (rfi *runtimeFnInfo) init(tm *llvmTypeMap, m llvm.Module, name string, args []types.Type, results []types.Type) {
	rfi.fi = new(functionTypeInfo)
	*rfi.fi = tm.getFunctionTypeInfo(args, results)
	rfi.fn = rfi.fi.declare(m, name)
}

func (rfi *runtimeFnInfo) call(f *frame, args ...llvm.Value) []llvm.Value {
	return rfi.fi.call(f.llvmtypes.ctx, f.allocaBuilder, f.builder, rfi.fn, args)
}

// runtimeInterface is a struct containing references to
// runtime types and intrinsic function declarations.
type runtimeInterface struct {
	// runtime types
	eface,
	rtype,
	uncommonType,
	arrayType,
	chanType,
	funcType,
	iface,
	imethod,
	interfaceType,
	itab,
	mapiter,
	mapType,
	method,
	ptrType,
	sliceType,
	structField,
	structType,
	defers runtimeType

	// intrinsics
	chanclose,
	chanrecv,
	chansend,
	convertE2I,
	convertE2V,
	mustConvertE2I,
	mustConvertE2V,
	eqtyp,
	initdefers,
	main,
	printfloat,
	makemap,
	makechan,
	malloc,
	mapaccess,
	mapiterinit,
	mapiternext,
	maplookup,
	memequal,
	panic_,
	pushdefer,
	recover_,
	rundefers,
	chancap,
	chanlen,
	runestostr,
	selectdefault,
	selectgo,
	selectinit,
	selectrecv,
	selectsend,
	selectsize,
	sliceappend,
	slicecopy,
	streqalg,
	stringslice,
	strnext,
	strrune,
	strtorunes,
	f32eqalg,
	f64eqalg,
	c64eqalg,
	c128eqalg *LLVMValue

	// LLVM intrinsics
	memcpy,
	memset llvm.Value

	checkInterfaceType,
	emptyInterfaceCompare,
	Go,
	interfaceCompare,
	makeSlice,
	mapdelete,
	mapIndex,
	mapLen,
	New,
	newMap,
	NewNopointers,
	printBool,
	printDouble,
	printEmptyInterface,
	printInterface,
	printInt64,
	printNl,
	printPointer,
	printSlice,
	printSpace,
	printString,
	printUint64,
	runtimeError,
	strcmp,
	stringPlus,
	typeDescriptorsEqual runtimeFnInfo
}

func newRuntimeInterface(pkg *types.Package, module llvm.Module, tm *llvmTypeMap, fr FuncResolver) (*runtimeInterface, error) {
	var ri runtimeInterface
	runtimeTypes := map[string]*runtimeType{
		"eface":         &ri.eface,
		"rtype":         &ri.rtype,
		"uncommonType":  &ri.uncommonType,
		"arrayType":     &ri.arrayType,
		"chanType":      &ri.chanType,
		"funcType":      &ri.funcType,
		"iface":         &ri.iface,
		"imethod":       &ri.imethod,
		"interfaceType": &ri.interfaceType,
		"itab":          &ri.itab,
		"mapiter":       &ri.mapiter,
		"mapType":       &ri.mapType,
		"method":        &ri.method,
		"ptrType":       &ri.ptrType,
		"sliceType":     &ri.sliceType,
		"structField":   &ri.structField,
		"structType":    &ri.structType,
		"defers":        &ri.defers,
	}
	for name, field := range runtimeTypes {
		obj := pkg.Scope().Lookup(name)
		if obj == nil {
			return nil, fmt.Errorf("no runtime type with name %s", name)
		}
		field.Type = obj.Type()
		field.llvm = tm.ToLLVM(field.Type)
	}

	intrinsics := map[string]**LLVMValue{
		"chanclose":         &ri.chanclose,
		"chanrecv":          &ri.chanrecv,
		"chansend":          &ri.chansend,
		"convertE2I":        &ri.convertE2I,
		"convertE2V":        &ri.convertE2V,
		"mustConvertE2I":    &ri.mustConvertE2I,
		"mustConvertE2V":    &ri.mustConvertE2V,
		"eqtyp":             &ri.eqtyp,
		"initdefers":        &ri.initdefers,
		"main":              &ri.main,
		"printfloat":        &ri.printfloat,
		"makechan":          &ri.makechan,
		"makemap":           &ri.makemap,
		"malloc":            &ri.malloc,
		"mapaccess":         &ri.mapaccess,
		"mapiterinit":       &ri.mapiterinit,
		"mapiternext":       &ri.mapiternext,
		"maplookup":         &ri.maplookup,
		"memequal":          &ri.memequal,
		"panic_":            &ri.panic_,
		"pushdefer":         &ri.pushdefer,
		"recover_":          &ri.recover_,
		"rundefers":         &ri.rundefers,
		"chancap":           &ri.chancap,
		"chanlen":           &ri.chanlen,
		"selectdefault":     &ri.selectdefault,
		"selectgo":          &ri.selectgo,
		"selectinit":        &ri.selectinit,
		"selectrecv":        &ri.selectrecv,
		"selectsend":        &ri.selectsend,
		"selectsize":        &ri.selectsize,
		"sliceappend":       &ri.sliceappend,
		"slicecopy":         &ri.slicecopy,
		"stringslice":       &ri.stringslice,
		"strnext":           &ri.strnext,
		"strrune":           &ri.strrune,
		"strtorunes":        &ri.strtorunes,
		"runestostr":        &ri.runestostr,
		"streqalg":          &ri.streqalg,
		"f32eqalg":          &ri.f32eqalg,
		"f64eqalg":          &ri.f64eqalg,
		"c64eqalg":          &ri.c64eqalg,
		"c128eqalg":         &ri.c128eqalg,
	}
	for name, field := range intrinsics {
		obj := pkg.Scope().Lookup(name)
		if obj == nil {
			return nil, fmt.Errorf("no runtime function with name %s", name)
		}
		*field = fr.ResolveFunc(obj.(*types.Func))
	}

	emptyInterface := types.NewInterface(nil, nil)
	intSlice := types.NewSlice(types.Typ[types.Int])
	for _, rt := range [...]struct {
		name          string
		rfi           *runtimeFnInfo
		args, results []types.Type
	}{
		{name: "__go_check_interface_type", rfi: &ri.checkInterfaceType, args: []types.Type{types.Typ[types.UnsafePointer], types.Typ[types.UnsafePointer], types.Typ[types.UnsafePointer]}},
		{name: "__go_empty_interface_compare", rfi: &ri.emptyInterfaceCompare, args: []types.Type{emptyInterface, emptyInterface}, results: []types.Type{types.Typ[types.Int]}},
		{name: "__go_go", rfi: &ri.Go, args: []types.Type{types.Typ[types.UnsafePointer], types.Typ[types.UnsafePointer]}},
		{name: "__go_interface_compare", rfi: &ri.interfaceCompare, args: []types.Type{emptyInterface, emptyInterface}, results: []types.Type{types.Typ[types.Int]}},
		{name: "__go_new_map", rfi: &ri.newMap, args: []types.Type{types.Typ[types.UnsafePointer], types.Typ[types.Uintptr]}, results: []types.Type{types.Typ[types.UnsafePointer]}},
		{name: "__go_make_slice2", rfi: &ri.makeSlice, args: []types.Type{types.Typ[types.UnsafePointer], types.Typ[types.Uintptr], types.Typ[types.Uintptr]}, results: []types.Type{intSlice}},
		{name: "runtime.mapdelete", rfi: &ri.mapdelete, args: []types.Type{types.Typ[types.UnsafePointer], types.Typ[types.UnsafePointer]}},
		{name: "__go_map_index", rfi: &ri.mapIndex, args: []types.Type{types.Typ[types.UnsafePointer], types.Typ[types.UnsafePointer], types.Typ[types.Bool]}, results: []types.Type{types.Typ[types.UnsafePointer]}},
		{name: "__go_map_len", rfi: &ri.mapLen, args: []types.Type{types.Typ[types.UnsafePointer]}, results: []types.Type{types.Typ[types.Int]}},
		{name: "__go_new", rfi: &ri.New, args: []types.Type{types.Typ[types.Uintptr]}, results: []types.Type{types.Typ[types.UnsafePointer]}},
		{name: "__go_new_nopointers", rfi: &ri.NewNopointers, args: []types.Type{types.Typ[types.Uintptr]}, results: []types.Type{types.Typ[types.UnsafePointer]}},
		{name: "__go_print_bool", rfi: &ri.printBool, args: []types.Type{types.Typ[types.Bool]}},
		{name: "__go_print_double", rfi: &ri.printDouble, args: []types.Type{types.Typ[types.Float64]}},
		{name: "__go_print_empty_interface", rfi: &ri.printEmptyInterface, args: []types.Type{emptyInterface}},
		{name: "__go_print_interface", rfi: &ri.printInterface, args: []types.Type{emptyInterface}},
		{name: "__go_print_int64", rfi: &ri.printInt64, args: []types.Type{types.Typ[types.Int64]}},
		{name: "__go_print_nl", rfi: &ri.printNl},
		{name: "__go_print_pointer", rfi: &ri.printPointer, args: []types.Type{types.Typ[types.UnsafePointer]}},
		{name: "__go_print_slice", rfi: &ri.printSlice, args: []types.Type{intSlice}},
		{name: "__go_print_space", rfi: &ri.printSpace},
		{name: "__go_print_string", rfi: &ri.printString, args: []types.Type{types.Typ[types.String]}},
		{name: "__go_print_uint64", rfi: &ri.printUint64, args: []types.Type{types.Typ[types.Int64]}},
		{name: "__go_runtime_error", rfi: &ri.runtimeError, args: []types.Type{types.Typ[types.Int32]}},
		{name: "__go_strcmp", rfi: &ri.strcmp, args: []types.Type{types.Typ[types.String], types.Typ[types.String]}, results: []types.Type{types.Typ[types.Int]}},
		{name: "__go_string_plus", rfi: &ri.stringPlus, args: []types.Type{types.Typ[types.String], types.Typ[types.String]}, results: []types.Type{types.Typ[types.String]}},
		{name: "__go_type_descriptors_equal", rfi: &ri.typeDescriptorsEqual, args: []types.Type{types.Typ[types.UnsafePointer], types.Typ[types.UnsafePointer]}, results: []types.Type{types.Typ[types.Bool]}},
	} {
		rt.rfi.init(tm, module, rt.name, rt.args, rt.results)
	}

	memsetName := "llvm.memset.p0i8.i" + strconv.Itoa(tm.target.IntPtrType().IntTypeWidth())
	memsetType := llvm.FunctionType(llvm.VoidType(), []llvm.Type{llvm.PointerType(llvm.Int8Type(), 0), llvm.Int8Type(), tm.target.IntPtrType(), llvm.Int32Type(), llvm.Int1Type()}, false)
	ri.memset = llvm.AddFunction(module, memsetName, memsetType)

	memcpyName := "llvm.memcpy.p0i8.p0i8.i" + strconv.Itoa(tm.target.IntPtrType().IntTypeWidth())
	memcpyType := llvm.FunctionType(llvm.VoidType(), []llvm.Type{llvm.PointerType(llvm.Int8Type(), 0), llvm.PointerType(llvm.Int8Type(), 0), tm.target.IntPtrType(), llvm.Int32Type(), llvm.Int1Type()}, false)
	ri.memcpy = llvm.AddFunction(module, memcpyName, memcpyType)

	return &ri, nil
}

// importRuntime locates the the runtime package and parses its files
// to *ast.Files. This is used to generate runtime type structures.
func parseRuntime(buildctx *build.Context, fset *token.FileSet) ([]*ast.File, error) {
	buildpkg, err := buildctx.Import("github.com/axw/llgo/pkg/runtime", "", 0)
	if err != nil {
		return nil, err
	}
	filenames := make([]string, len(buildpkg.GoFiles))
	for i, f := range buildpkg.GoFiles {
		filenames[i] = path.Join(buildpkg.Dir, f)
	}
	return parseFiles(fset, filenames)
}

func (fr *frame) createZExtOrTrunc(v llvm.Value, t llvm.Type, name string) llvm.Value {
	switch n := v.Type().IntTypeWidth() - t.IntTypeWidth(); {
	case n < 0:
		v = fr.builder.CreateZExt(v, fr.target.IntPtrType(), name)
	case n > 0:
		v = fr.builder.CreateTrunc(v, fr.target.IntPtrType(), name)
	}
	return v
}

func (fr *frame) createMalloc(size llvm.Value, hasPointers bool) llvm.Value {
	var allocator *runtimeFnInfo
	if hasPointers {
		allocator = &fr.runtime.New
	} else {
		allocator = &fr.runtime.NewNopointers
	}

	return allocator.call(fr, fr.createZExtOrTrunc(size, fr.target.IntPtrType(), ""))[0]
}

func (fr *frame) createTypeMalloc(t types.Type) llvm.Value {
	size := llvm.ConstInt(fr.target.IntPtrType(), uint64(fr.llvmtypes.Sizeof(t)), false)
	malloc := fr.createMalloc(size, hasPointers(t))
	return fr.builder.CreateBitCast(malloc, llvm.PointerType(fr.types.ToLLVM(t), 0), "")
}

func (fr *frame) memsetZero(ptr llvm.Value, size llvm.Value) {
	memset := fr.runtime.memset
	ptr = fr.builder.CreateBitCast(ptr, llvm.PointerType(llvm.Int8Type(), 0), "")
	fill := llvm.ConstNull(llvm.Int8Type())
	size = fr.createZExtOrTrunc(size, fr.target.IntPtrType(), "")
	align := llvm.ConstInt(llvm.Int32Type(), 1, false)
	isvolatile := llvm.ConstNull(llvm.Int1Type())
	fr.builder.CreateCall(memset, []llvm.Value{ptr, fill, size, align, isvolatile}, "")
}

func (fr *frame) memcpy(dest llvm.Value, src llvm.Value, size llvm.Value) {
	memcpy := fr.runtime.memcpy
	dest = fr.builder.CreateBitCast(dest, llvm.PointerType(llvm.Int8Type(), 0), "")
	src = fr.builder.CreateBitCast(src, llvm.PointerType(llvm.Int8Type(), 0), "")
	size = fr.createZExtOrTrunc(size, fr.target.IntPtrType(), "")
	align := llvm.ConstInt(llvm.Int32Type(), 1, false)
	isvolatile := llvm.ConstNull(llvm.Int1Type())
	fr.builder.CreateCall(memcpy, []llvm.Value{dest, src, size, align, isvolatile}, "")
}

func (c *compiler) stacksave() llvm.Value {
	return llvm.Value{C: nil}
}

func (c *compiler) stackrestore(ctx llvm.Value) {
}
