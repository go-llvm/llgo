// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
	"go/ast"
)

// makeLiteralSlice allocates a new slice, storing in it the provided elements.
func (c *compiler) makeLiteralSlice(v []llvm.Value, elttyp types.Type) llvm.Value {
	n := llvm.ConstInt(c.types.inttype, uint64(len(v)), false)
	eltType := c.types.ToLLVM(elttyp)
	arrayType := llvm.ArrayType(eltType, len(v))
	mem := c.createMalloc(llvm.SizeOf(arrayType))
	mem = c.builder.CreateIntToPtr(mem, llvm.PointerType(eltType, 0), "")
	for i, value := range v {
		indices := []llvm.Value{llvm.ConstInt(llvm.Int32Type(), uint64(i), false)}
		ep := c.builder.CreateGEP(mem, indices, "")
		c.builder.CreateStore(value, ep)
	}
	slicetyp := types.NewSlice(elttyp)
	struct_ := llvm.Undef(c.types.ToLLVM(slicetyp))
	struct_ = c.builder.CreateInsertValue(struct_, mem, 0, "")
	struct_ = c.builder.CreateInsertValue(struct_, n, 1, "")
	struct_ = c.builder.CreateInsertValue(struct_, n, 2, "")
	return struct_
}

// makeSlice allocates a new slice with the optional length and capacity,
// initialising its contents to their zero values.
func (c *compiler) makeSlice(elttyp types.Type, length, capacity Value) llvm.Value {
	var lengthValue llvm.Value
	if length != nil {
		lengthValue = length.Convert(types.Typ[types.Int]).LLVMValue()
	} else {
		lengthValue = llvm.ConstNull(c.llvmtypes.inttype)
	}

	// TODO check capacity >= length
	capacityValue := lengthValue
	if capacity != nil {
		capacityValue = capacity.Convert(types.Typ[types.Int]).LLVMValue()
	}

	eltType := c.types.ToLLVM(elttyp)
	sizeof := llvm.ConstTruncOrBitCast(llvm.SizeOf(eltType), c.types.inttype)
	size := c.builder.CreateMul(capacityValue, sizeof, "")
	mem := c.createMalloc(size)
	mem = c.builder.CreateIntToPtr(mem, llvm.PointerType(eltType, 0), "")
	c.memsetZero(mem, size)

	slicetyp := types.NewSlice(elttyp)
	struct_ := llvm.Undef(c.types.ToLLVM(slicetyp))
	struct_ = c.builder.CreateInsertValue(struct_, mem, 0, "")
	struct_ = c.builder.CreateInsertValue(struct_, lengthValue, 1, "")
	struct_ = c.builder.CreateInsertValue(struct_, capacityValue, 2, "")
	return struct_
}

// coerceSlice takes a slice of one element type and coerces it to a
// slice of another.
func (c *compiler) coerceSlice(src llvm.Value, dsttyp llvm.Type) llvm.Value {
	dst := llvm.Undef(dsttyp)
	srcmem := c.builder.CreateExtractValue(src, 0, "")
	srclen := c.builder.CreateExtractValue(src, 1, "")
	srccap := c.builder.CreateExtractValue(src, 2, "")
	dstmemtyp := dsttyp.StructElementTypes()[0]
	dstmem := c.builder.CreateBitCast(srcmem, dstmemtyp, "")
	dst = c.builder.CreateInsertValue(dst, dstmem, 0, "")
	dst = c.builder.CreateInsertValue(dst, srclen, 1, "")
	dst = c.builder.CreateInsertValue(dst, srccap, 2, "")
	return dst
}

func (c *compiler) VisitAppend(expr *ast.CallExpr) Value {
	s := c.VisitExpr(expr.Args[0])
	elemtyp := s.Type().Underlying().(*types.Slice).Elem()
	if len(expr.Args) == 1 {
		return s
	} else if expr.Ellipsis.IsValid() {
		c.convertUntyped(expr.Args[1], s.Type())
	} else {
		for _, arg := range expr.Args[1:] {
			c.convertUntyped(arg, elemtyp)
		}
	}

	sliceappend := c.NamedFunction("runtime.sliceappend", "func(t uintptr, dst, src slice) slice")
	i8slice := sliceappend.Type().ElementType().ReturnType()
	i8ptr := c.types.ToLLVM(types.NewPointer(types.Typ[types.Int8]))

	// Coerce first argument into an []int8.
	a_ := s.LLVMValue()
	sliceTyp := a_.Type()
	a := c.coerceSlice(a_, i8slice)

	var b llvm.Value
	if expr.Ellipsis.IsValid() {
		// Pass the provided slice straight through. If it's a string,
		// convert it to a []byte first.
		elem := c.VisitExpr(expr.Args[1]).Convert(s.Type())
		b = c.coerceSlice(elem.LLVMValue(), i8slice)
	} else {
		// Construct a fresh []int8 for the temporary slice.
		n := llvm.ConstInt(c.types.inttype, uint64(len(expr.Args)-1), false)
		mem := c.builder.CreateArrayAlloca(c.types.ToLLVM(elemtyp), n, "")
		for i, arg := range expr.Args[1:] {
			elem := c.VisitExpr(arg).Convert(elemtyp)
			indices := []llvm.Value{llvm.ConstInt(llvm.Int32Type(), uint64(i), false)}
			ptr := c.builder.CreateGEP(mem, indices, "")
			c.builder.CreateStore(elem.LLVMValue(), ptr)
		}
		b = llvm.Undef(i8slice)
		b = c.builder.CreateInsertValue(b, c.builder.CreateBitCast(mem, i8ptr, ""), 0, "")
		b = c.builder.CreateInsertValue(b, n, 1, "")
		b = c.builder.CreateInsertValue(b, n, 2, "")
	}

	// Call runtime function, then coerce the result.
	runtimeTyp := c.types.ToRuntime(s.Type())
	runtimeTyp = c.builder.CreatePtrToInt(runtimeTyp, c.target.IntPtrType(), "")
	args := []llvm.Value{runtimeTyp, a, b}
	result := c.builder.CreateCall(sliceappend, args, "")
	return c.NewValue(c.coerceSlice(result, sliceTyp), s.Type())
}

func (c *compiler) VisitCopy(expr *ast.CallExpr) Value {
	dest := c.VisitExpr(expr.Args[0])
	source := c.VisitExpr(expr.Args[1])

	// If it's a string, convert it to a []byte first.
	source = source.Convert(dest.Type())

	slicecopy := c.NamedFunction("runtime.slicecopy", "func(t uintptr, dst, src slice) int")
	i8slice := slicecopy.Type().ElementType().ParamTypes()[1]

	// Coerce first argument into an []int8.
	dest_ := c.coerceSlice(dest.LLVMValue(), i8slice)
	source_ := c.coerceSlice(source.LLVMValue(), i8slice)

	// Call runtime function.
	runtimeTyp := c.types.ToRuntime(dest.Type())
	runtimeTyp = c.builder.CreatePtrToInt(runtimeTyp, c.target.IntPtrType(), "")
	args := []llvm.Value{runtimeTyp, dest_, source_}
	result := c.builder.CreateCall(slicecopy, args, "")
	return c.NewValue(result, types.Typ[types.Int])
}

func (c *compiler) VisitSliceExpr(expr *ast.SliceExpr) Value {
	value := c.VisitExpr(expr.X)

	var low llvm.Value
	if expr.Low != nil {
		lowvalue := c.VisitExpr(expr.Low).Convert(types.Typ[types.Int])
		low = lowvalue.LLVMValue()
	} else {
		low = llvm.ConstNull(c.types.inttype)
	}

	var high llvm.Value
	if expr.High != nil {
		highvalue := c.VisitExpr(expr.High).Convert(types.Typ[types.Int])
		high = highvalue.LLVMValue()
	} else {
		high = llvm.ConstAllOnes(c.types.inttype) // -1
	}

	if _, ok := value.Type().Underlying().(*types.Pointer); ok {
		value = value.(*LLVMValue).makePointee()
	}

	switch typ := value.Type().Underlying().(type) {
	case *types.Array:
		sliceslice := c.NamedFunction("runtime.sliceslice", "func(t uintptr, s slice, low, high int) slice")
		i8slice := sliceslice.Type().ElementType().ReturnType()
		sliceValue := llvm.Undef(i8slice) // temporary slice
		arrayptr := value.(*LLVMValue).pointer.LLVMValue()
		arrayptr = c.builder.CreateBitCast(arrayptr, i8slice.StructElementTypes()[0], "")
		arraylen := llvm.ConstInt(c.llvmtypes.inttype, uint64(typ.Len()), false)
		sliceValue = c.builder.CreateInsertValue(sliceValue, arrayptr, 0, "")
		sliceValue = c.builder.CreateInsertValue(sliceValue, arraylen, 1, "")
		sliceValue = c.builder.CreateInsertValue(sliceValue, arraylen, 2, "")
		sliceTyp := types.NewSlice(typ.Elem())
		runtimeTyp := c.types.ToRuntime(sliceTyp)
		runtimeTyp = c.builder.CreatePtrToInt(runtimeTyp, c.target.IntPtrType(), "")
		args := []llvm.Value{runtimeTyp, sliceValue, low, high}
		result := c.builder.CreateCall(sliceslice, args, "")
		llvmSliceTyp := c.types.ToLLVM(sliceTyp)
		return c.NewValue(c.coerceSlice(result, llvmSliceTyp), sliceTyp)
	case *types.Slice:
		sliceslice := c.NamedFunction("runtime.sliceslice", "func(t uintptr, s slice, low, high int) slice")
		i8slice := sliceslice.Type().ElementType().ReturnType()
		sliceValue := value.LLVMValue()
		sliceTyp := sliceValue.Type()
		sliceValue = c.coerceSlice(sliceValue, i8slice)
		runtimeTyp := c.types.ToRuntime(value.Type())
		runtimeTyp = c.builder.CreatePtrToInt(runtimeTyp, c.target.IntPtrType(), "")
		args := []llvm.Value{runtimeTyp, sliceValue, low, high}
		result := c.builder.CreateCall(sliceslice, args, "")
		return c.NewValue(c.coerceSlice(result, sliceTyp), value.Type())
	case *types.Basic:
		stringslice := c.NamedFunction("runtime.stringslice", "func(a string, low, high int) string")
		args := []llvm.Value{value.LLVMValue(), low, high}
		result := c.builder.CreateCall(stringslice, args, "")
		return c.NewValue(result, value.Type())
	default:
		panic("unimplemented")
	}
	panic("unreachable")
}

// vim: set ft=go :
