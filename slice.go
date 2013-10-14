// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
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
func (c *compiler) makeSlice(sliceType types.Type, length, capacity *LLVMValue) *LLVMValue {
	// TODO check capacity >= length
	lengthValue := length.LLVMValue()
	capacityValue := capacity.LLVMValue()

	elemType := c.types.ToLLVM(sliceType.Underlying().(*types.Slice).Elem())
	sizeof := llvm.ConstTruncOrBitCast(llvm.SizeOf(elemType), c.types.inttype)
	size := c.builder.CreateMul(capacityValue, sizeof, "")
	mem := c.createMalloc(size)
	mem = c.builder.CreateIntToPtr(mem, llvm.PointerType(elemType, 0), "")
	c.memsetZero(mem, size)

	slice := llvm.Undef(c.types.ToLLVM(sliceType))
	slice = c.builder.CreateInsertValue(slice, mem, 0, "")
	slice = c.builder.CreateInsertValue(slice, lengthValue, 1, "")
	slice = c.builder.CreateInsertValue(slice, capacityValue, 2, "")
	return c.NewValue(slice, sliceType)
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

/*
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

	sliceappend := c.RuntimeFunction("runtime.sliceappend", "func(t uintptr, dst, src slice) slice")
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

	slicecopy := c.RuntimeFunction("runtime.slicecopy", "func(t uintptr, dst, src slice) int")
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
*/

func (c *compiler) slice(x, low, high *LLVMValue) *LLVMValue {
	if low != nil {
		low = low.Convert(types.Typ[types.Int]).(*LLVMValue)
	} else {
		low = c.NewValue(llvm.ConstNull(c.types.inttype), types.Typ[types.Int])
	}

	if high != nil {
		high = high.Convert(types.Typ[types.Int]).(*LLVMValue)
	} else {
		// all bits set is -1
		high = c.NewValue(llvm.ConstAllOnes(c.types.inttype), types.Typ[types.Int])
	}

	switch typ := x.Type().Underlying().(type) {
	case *types.Pointer: // *array
		sliceslice := c.RuntimeFunction("runtime.sliceslice", "func(t uintptr, s slice, low, high int) slice")
		i8slice := sliceslice.Type().ElementType().ReturnType()
		sliceValue := llvm.Undef(i8slice) // temporary slice
		arraytyp := typ.Elem().(*types.Array)
		arrayptr := x.LLVMValue()
		arrayptr = c.builder.CreateBitCast(arrayptr, i8slice.StructElementTypes()[0], "")
		arraylen := llvm.ConstInt(c.llvmtypes.inttype, uint64(arraytyp.Len()), false)
		sliceValue = c.builder.CreateInsertValue(sliceValue, arrayptr, 0, "")
		sliceValue = c.builder.CreateInsertValue(sliceValue, arraylen, 1, "")
		sliceValue = c.builder.CreateInsertValue(sliceValue, arraylen, 2, "")
		sliceTyp := types.NewSlice(arraytyp.Elem())
		runtimeTyp := c.types.ToRuntime(sliceTyp)
		runtimeTyp = c.builder.CreatePtrToInt(runtimeTyp, c.target.IntPtrType(), "")
		args := []llvm.Value{runtimeTyp, sliceValue, low.LLVMValue(), high.LLVMValue()}
		result := c.builder.CreateCall(sliceslice, args, "")
		llvmSliceTyp := c.types.ToLLVM(sliceTyp)
		return c.NewValue(c.coerceSlice(result, llvmSliceTyp), sliceTyp)
	case *types.Slice:
		sliceslice := c.RuntimeFunction("runtime.sliceslice", "func(t uintptr, s slice, low, high int) slice")
		i8slice := sliceslice.Type().ElementType().ReturnType()
		sliceValue := x.LLVMValue()
		sliceTyp := sliceValue.Type()
		sliceValue = c.coerceSlice(sliceValue, i8slice)
		runtimeTyp := c.types.ToRuntime(x.Type())
		runtimeTyp = c.builder.CreatePtrToInt(runtimeTyp, c.target.IntPtrType(), "")
		args := []llvm.Value{runtimeTyp, sliceValue, low.LLVMValue(), high.LLVMValue()}
		result := c.builder.CreateCall(sliceslice, args, "")
		return c.NewValue(c.coerceSlice(result, sliceTyp), x.Type())
	case *types.Basic:
		stringslice := c.RuntimeFunction("runtime.stringslice", "func(a string, low, high int) string")
		args := []llvm.Value{x.LLVMValue(), low.LLVMValue(), high.LLVMValue()}
		result := c.builder.CreateCall(stringslice, args, "")
		return c.NewValue(result, x.Type())
	default:
		panic("unimplemented")
	}
	panic("unreachable")
}
