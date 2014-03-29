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

	"github.com/go-llvm/llvm"
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
	// intrinsics
	chanrecv,
	chansend,
	chancap,
	chanlen,
	selectdefault,
	selectgo,
	selectinit,
	selectrecv,
	selectsend,
	selectsize *LLVMValue

	// LLVM intrinsics
	memcpy,
	memset llvm.Value

	append,
	assertInterface,
	checkInterfaceType,
	convertInterface,
	copy,
	emptyInterfaceCompare,
	Go,
	ifaceE2I2,
	ifaceI2I2,
	intArrayToString,
	interfaceCompare,
	intToString,
	makeSlice,
	mapdelete,
	mapiter2,
	mapiterinit,
	mapiternext,
	mapIndex,
	mapLen,
	New,
	newChannel,
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
	stringiter2,
	stringPlus,
	stringSlice,
	stringToIntArray,
	typeDescriptorsEqual runtimeFnInfo
}

func newRuntimeInterface(pkg *types.Package, module llvm.Module, tm *llvmTypeMap, fr FuncResolver) (*runtimeInterface, error) {
	var ri runtimeInterface
	intrinsics := map[string]**LLVMValue{
		"chanrecv":      &ri.chanrecv,
		"chansend":      &ri.chansend,
		"chancap":       &ri.chancap,
		"chanlen":       &ri.chanlen,
		"selectdefault": &ri.selectdefault,
		"selectgo":      &ri.selectgo,
		"selectinit":    &ri.selectinit,
		"selectrecv":    &ri.selectrecv,
		"selectsend":    &ri.selectsend,
		"selectsize":    &ri.selectsize,
	}
	for name, field := range intrinsics {
		obj := pkg.Scope().Lookup(name)
		if obj == nil {
			return nil, fmt.Errorf("no runtime function with name %s", name)
		}
		*field = fr.ResolveFunc(obj.(*types.Func))
	}

	Bool := types.Typ[types.Bool]
	Float64 := types.Typ[types.Float64]
	Int32 := types.Typ[types.Int32]
	Int64 := types.Typ[types.Int64]
	Int := types.Typ[types.Int]
	Rune := types.Typ[types.Rune]
	String := types.Typ[types.String]
	Uintptr := types.Typ[types.Uintptr]
	UnsafePointer := types.Typ[types.UnsafePointer]

	EmptyInterface := types.NewInterface(nil, nil)
	IntSlice := types.NewSlice(types.Typ[types.Int])

	for _, rt := range [...]struct {
		name      string
		rfi       *runtimeFnInfo
		args, res []types.Type
	}{
		{
			name: "__go_append",
			rfi:  &ri.append,
			args: []types.Type{IntSlice, UnsafePointer, Uintptr, Uintptr},
			res:  []types.Type{IntSlice},
		},
		{
			name: "__go_assert_interface",
			rfi:  &ri.assertInterface,
			args: []types.Type{UnsafePointer, UnsafePointer},
			res:  []types.Type{UnsafePointer},
		},
		{
			name: "__go_check_interface_type",
			rfi:  &ri.checkInterfaceType,
			args: []types.Type{UnsafePointer, UnsafePointer, UnsafePointer},
		},
		{
			name: "__go_convert_interface",
			rfi:  &ri.convertInterface,
			args: []types.Type{UnsafePointer, UnsafePointer},
			res:  []types.Type{UnsafePointer},
		},
		{
			name: "__go_copy",
			rfi:  &ri.copy,
			args: []types.Type{UnsafePointer, UnsafePointer, Uintptr},
		},
		{
			name: "__go_empty_interface_compare",
			rfi:  &ri.emptyInterfaceCompare,
			args: []types.Type{EmptyInterface, EmptyInterface},
			res:  []types.Type{Int},
		},
		{
			name: "__go_go",
			rfi:  &ri.Go,
			args: []types.Type{UnsafePointer, UnsafePointer},
		},
		{
			name: "runtime.ifaceE2I2",
			rfi:  &ri.ifaceE2I2,
			args: []types.Type{UnsafePointer, EmptyInterface},
			res:  []types.Type{EmptyInterface, Bool},
		},
		{
			name: "runtime.ifaceI2I2",
			rfi:  &ri.ifaceI2I2,
			args: []types.Type{UnsafePointer, EmptyInterface},
			res:  []types.Type{EmptyInterface, Bool},
		},
		{
			name: "__go_int_array_to_string",
			rfi:  &ri.intArrayToString,
			args: []types.Type{UnsafePointer, Int},
			res:  []types.Type{String},
		},
		{
			name: "__go_int_to_string",
			rfi:  &ri.intToString,
			args: []types.Type{Int},
			res:  []types.Type{String},
		},
		{
			name: "__go_interface_compare",
			rfi:  &ri.interfaceCompare,
			args: []types.Type{EmptyInterface, EmptyInterface},
			res:  []types.Type{Int},
		},
		{
			name: "__go_make_slice2",
			rfi:  &ri.makeSlice,
			args: []types.Type{UnsafePointer, Uintptr, Uintptr},
			res:  []types.Type{IntSlice},
		},
		{
			name: "runtime.mapdelete",
			rfi:  &ri.mapdelete,
			args: []types.Type{UnsafePointer, UnsafePointer},
		},
		{
			name: "runtime.mapiter2",
			rfi:  &ri.mapiter2,
			args: []types.Type{UnsafePointer, UnsafePointer, UnsafePointer},
		},
		{
			name: "runtime.mapiterinit",
			rfi:  &ri.mapiterinit,
			args: []types.Type{UnsafePointer, UnsafePointer},
		},
		{
			name: "runtime.mapiternext",
			rfi:  &ri.mapiternext,
			args: []types.Type{UnsafePointer},
		},
		{
			name: "__go_map_index",
			rfi:  &ri.mapIndex,
			args: []types.Type{UnsafePointer, UnsafePointer, Bool},
			res:  []types.Type{UnsafePointer},
		},
		{
			name: "__go_map_len",
			rfi:  &ri.mapLen,
			args: []types.Type{UnsafePointer},
			res:  []types.Type{Int},
		},
		{
			name: "__go_new",
			rfi:  &ri.New,
			args: []types.Type{Uintptr},
			res:  []types.Type{UnsafePointer},
		},
		{
			name: "__go_new_channel",
			rfi:  &ri.newChannel,
			args: []types.Type{UnsafePointer, Uintptr},
			res:  []types.Type{UnsafePointer},
		},
		{
			name: "__go_new_map",
			rfi:  &ri.newMap,
			args: []types.Type{UnsafePointer, Uintptr},
			res:  []types.Type{UnsafePointer},
		},
		{
			name: "__go_new_nopointers",
			rfi:  &ri.NewNopointers,
			args: []types.Type{Uintptr},
			res:  []types.Type{UnsafePointer},
		},
		{
			name: "__go_print_bool",
			rfi:  &ri.printBool,
			args: []types.Type{Bool},
		},
		{
			name: "__go_print_double",
			rfi:  &ri.printDouble,
			args: []types.Type{Float64},
		},
		{
			name: "__go_print_empty_interface",
			rfi:  &ri.printEmptyInterface,
			args: []types.Type{EmptyInterface},
		},
		{
			name: "__go_print_interface",
			rfi:  &ri.printInterface,
			args: []types.Type{EmptyInterface},
		},
		{
			name: "__go_print_int64",
			rfi:  &ri.printInt64,
			args: []types.Type{Int64},
		},
		{
			name: "__go_print_nl",
			rfi:  &ri.printNl,
		},
		{
			name: "__go_print_pointer",
			rfi:  &ri.printPointer,
			args: []types.Type{UnsafePointer},
		},
		{
			name: "__go_print_slice",
			rfi:  &ri.printSlice,
			args: []types.Type{IntSlice},
		},
		{
			name: "__go_print_space",
			rfi:  &ri.printSpace,
		},
		{
			name: "__go_print_string",
			rfi:  &ri.printString,
			args: []types.Type{String},
		},
		{
			name: "__go_print_uint64",
			rfi:  &ri.printUint64,
			args: []types.Type{Int64},
		},
		{
			name: "__go_runtime_error",
			rfi:  &ri.runtimeError,
			args: []types.Type{Int32},
		},
		{
			name: "__go_strcmp",
			rfi:  &ri.strcmp,
			args: []types.Type{String, String},
			res:  []types.Type{Int},
		},
		{
			name: "__go_string_plus",
			rfi:  &ri.stringPlus,
			args: []types.Type{String, String},
			res:  []types.Type{String},
		},
		{
			name: "__go_string_slice",
			rfi:  &ri.stringSlice,
			args: []types.Type{String, Int, Int},
			res:  []types.Type{String},
		},
		{
			name: "__go_string_to_int_array",
			rfi:  &ri.stringToIntArray,
			args: []types.Type{String},
			res:  []types.Type{IntSlice},
		},
		{
			name: "runtime.stringiter2",
			rfi:  &ri.stringiter2,
			args: []types.Type{String, Int},
			res:  []types.Type{Int, Rune},
		},
		{
			name: "__go_type_descriptors_equal",
			rfi:  &ri.typeDescriptorsEqual,
			args: []types.Type{UnsafePointer, UnsafePointer},
			res:  []types.Type{Bool},
		},
	} {
		rt.rfi.init(tm, module, rt.name, rt.args, rt.res)
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
	buildpkg, err := buildctx.Import("github.com/go-llvm/llgo/pkg/runtime", "", 0)
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
