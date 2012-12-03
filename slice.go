/*
Copyright (c) 2012 Andrew Wilkins <axwalk@gmail.com>

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
	"github.com/axw/gollvm/llvm"
	"./types"
	"go/ast"
)

// makeLiteralSlice allocates a new slice, storing in it the provided elements.
func (c *compiler) makeLiteralSlice(v []llvm.Value, elttyp types.Type) llvm.Value {
	n := llvm.ConstInt(llvm.Int32Type(), uint64(len(v)), false)
	eltType := c.types.ToLLVM(elttyp)
	arrayType := llvm.ArrayType(eltType, len(v))
	mem := c.createMalloc(llvm.SizeOf(arrayType))
	mem = c.builder.CreateIntToPtr(mem, llvm.PointerType(eltType, 0), "")
	for i, value := range v {
		indices := []llvm.Value{llvm.ConstInt(llvm.Int32Type(), uint64(i), false)}
		ep := c.builder.CreateGEP(mem, indices, "")
		c.builder.CreateStore(value, ep)
	}
	slicetyp := types.Slice{Elt: elttyp}
	struct_ := llvm.Undef(c.types.ToLLVM(&slicetyp))
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
		lengthValue = length.Convert(types.Int32).LLVMValue()
	} else {
		lengthValue = llvm.ConstNull(llvm.Int32Type())
	}

	// TODO check capacity >= length
	capacityValue := lengthValue
	if capacity != nil {
		capacityValue = capacity.Convert(types.Int32).LLVMValue()
	}

	eltType := c.types.ToLLVM(elttyp)
	sizeof := llvm.ConstTrunc(llvm.SizeOf(eltType), llvm.Int32Type())
	size := c.builder.CreateMul(capacityValue, sizeof, "")
	mem := c.createMalloc(c.NewLLVMValue(size, types.Int32).Convert(types.Uintptr).LLVMValue())
	mem = c.builder.CreateIntToPtr(mem, llvm.PointerType(eltType, 0), "")
	c.memsetZero(mem, size)

	slicetyp := types.Slice{Elt: elttyp}
	struct_ := llvm.Undef(c.types.ToLLVM(&slicetyp))
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
	// TODO handle ellpisis arg
	s := c.VisitExpr(expr.Args[0])
	elem := c.VisitExpr(expr.Args[1])

	sliceappend := c.NamedFunction("runtime.sliceappend", "func f(t uintptr, dst, src slice) slice")
	i8slice := sliceappend.Type().ElementType().ReturnType()
	i8ptr := c.types.ToLLVM(&types.Pointer{Base: types.Int8})

	// Coerce first argument into an []int8.
	a_ := s.LLVMValue()
	sliceTyp := a_.Type()
	a := c.coerceSlice(a_, i8slice)

	var b llvm.Value
	if expr.Ellipsis.IsValid() {
		// Pass the provided slice straight through. If it's a string,
		// convert it to a []byte first.
		elem = elem.Convert(s.Type())
		b = c.coerceSlice(elem.LLVMValue(), i8slice)
	} else {
		// Construct a fresh []int8 for the temporary slice.
		b_ := elem.LLVMValue()
		one := llvm.ConstInt(llvm.Int32Type(), 1, false)
		mem := c.builder.CreateAlloca(elem.LLVMValue().Type(), "")
		c.builder.CreateStore(b_, mem)
		b = llvm.Undef(i8slice)
		b = c.builder.CreateInsertValue(b, c.builder.CreateBitCast(mem, i8ptr, ""), 0, "")
		b = c.builder.CreateInsertValue(b, one, 1, "")
		b = c.builder.CreateInsertValue(b, one, 2, "")
	}

	// Call runtime function, then coerce the result.
	runtimeTyp := c.types.ToRuntime(s.Type())
	runtimeTyp = c.builder.CreatePtrToInt(runtimeTyp, c.target.IntPtrType(), "")
	args := []llvm.Value{runtimeTyp, a, b}
	result := c.builder.CreateCall(sliceappend, args, "")
	return c.NewLLVMValue(c.coerceSlice(result, sliceTyp), s.Type())
}

func (c *compiler) VisitSliceExpr(expr *ast.SliceExpr) Value {
	// expr.X, expr.Low, expr.High
	value := c.VisitExpr(expr.X)
	var low, high llvm.Value
	if expr.Low != nil {
		low = c.VisitExpr(expr.Low).Convert(types.Int32).LLVMValue()
	} else {
		low = llvm.ConstNull(llvm.Int32Type())
	}
	if expr.High != nil {
		high = c.VisitExpr(expr.High).Convert(types.Int32).LLVMValue()
	} else {
		high = llvm.ConstAllOnes(llvm.Int32Type()) // -1
	}

	if _, ok := types.Underlying(value.Type()).(*types.Pointer); ok {
		value = value.(*LLVMValue).makePointee()
	}

	switch typ := types.Underlying(value.Type()).(type) {
	case *types.Array:
		sliceslice := c.NamedFunction("runtime.sliceslice", "func f(t uintptr, s slice, low, high int32) slice")
		i8slice := sliceslice.Type().ElementType().ReturnType()
		sliceValue := llvm.Undef(i8slice) // temporary slice
		arrayptr := value.(*LLVMValue).pointer.LLVMValue()
		arrayptr = c.builder.CreateBitCast(arrayptr, i8slice.StructElementTypes()[0], "")
		arraylen := llvm.ConstInt(llvm.Int32Type(), typ.Len, false)
		sliceValue = c.builder.CreateInsertValue(sliceValue, arrayptr, 0, "")
		sliceValue = c.builder.CreateInsertValue(sliceValue, arraylen, 1, "")
		sliceValue = c.builder.CreateInsertValue(sliceValue, arraylen, 2, "")
		sliceTyp := &types.Slice{Elt: typ.Elt}
		runtimeTyp := c.types.ToRuntime(sliceTyp)
		runtimeTyp = c.builder.CreatePtrToInt(runtimeTyp, c.target.IntPtrType(), "")
		args := []llvm.Value{runtimeTyp, sliceValue, low, high}
		result := c.builder.CreateCall(sliceslice, args, "")
		llvmSliceTyp := c.types.ToLLVM(sliceTyp)
		return c.NewLLVMValue(c.coerceSlice(result, llvmSliceTyp), sliceTyp)
	case *types.Slice:
		sliceslice := c.NamedFunction("runtime.sliceslice", "func f(t uintptr, s slice, low, high int32) slice")
		i8slice := sliceslice.Type().ElementType().ReturnType()
		sliceValue := value.LLVMValue()
		sliceTyp := sliceValue.Type()
		sliceValue = c.coerceSlice(sliceValue, i8slice)
		runtimeTyp := c.types.ToRuntime(value.Type())
		runtimeTyp = c.builder.CreatePtrToInt(runtimeTyp, c.target.IntPtrType(), "")
		args := []llvm.Value{runtimeTyp, sliceValue, low, high}
		result := c.builder.CreateCall(sliceslice, args, "")
		return c.NewLLVMValue(c.coerceSlice(result, sliceTyp), value.Type())
	case *types.Name: // String
		stringslice := c.NamedFunction("runtime.stringslice", "func f(a string, low, high int32) string")
		args := []llvm.Value{value.LLVMValue(), low, high}
		result := c.builder.CreateCall(stringslice, args, "")
		return c.NewLLVMValue(result, value.Type())
	default:
		panic("unimplemented")
	}
	panic("unreachable")
}

// vim: set ft=go :
