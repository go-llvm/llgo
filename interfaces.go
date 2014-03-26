// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// interfaceMethod returns a function and receiver pointer for the specified
// interface and method pair.
func (fr *frame) interfaceMethod(iface *LLVMValue, method *types.Func) (fn, recv *LLVMValue) {
	lliface := iface.LLVMValue()
	llitab := fr.builder.CreateExtractValue(lliface, 0, "")
	recv = fr.NewValue(fr.builder.CreateExtractValue(lliface, 1, ""), types.Typ[types.UnsafePointer])
	sig := method.Type().(*types.Signature)
	methodset := fr.types.MethodSet(sig.Recv().Type())
	// TODO(axw) cache ordered method index
	var index int
	for i := 0; i < methodset.Len(); i++ {
		if methodset.At(i).Obj() == method {
			index = i
			break
		}
	}
	llitab = fr.builder.CreateBitCast(llitab, llvm.PointerType(llvm.PointerType(llvm.Int8Type(), 0), 0), "")
	// Skip runtime type pointer.
	llifnptr := fr.builder.CreateGEP(llitab, []llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), uint64(index+1), false),
	}, "")

	llifn := fr.builder.CreateLoad(llifnptr, "")
	// Replace receiver type with unsafe.Pointer.
	recvparam := types.NewParam(0, nil, "", types.Typ[types.UnsafePointer])
	sig = types.NewSignature(nil, recvparam, sig.Params(), sig.Results(), sig.Variadic())
	fn = fr.NewValue(llifn, sig)
	return
}

// compareInterfaces emits code to compare two interfaces for
// equality.
func (fr *frame) compareInterfaces(a, b *LLVMValue) *LLVMValue {
	aNull := a.LLVMValue().IsNull()
	bNull := b.LLVMValue().IsNull()
	if aNull && bNull {
		return fr.NewValue(boolLLVMValue(true), types.Typ[types.Bool])
	}

	compare := fr.runtime.emptyInterfaceCompare
	aI := a.Type().Underlying().(*types.Interface).NumMethods() > 0
	bI := b.Type().Underlying().(*types.Interface).NumMethods() > 0
	switch {
	case aI && bI:
		compare = fr.runtime.interfaceCompare
	case aI:
		a = fr.convertI2E(a)
	case bI:
		b = fr.convertI2E(b)
	}

	result := compare.call(fr, a.LLVMValue(), b.LLVMValue())[0]
	result = fr.builder.CreateIsNull(result, "")
	result = fr.builder.CreateZExt(result, llvm.Int8Type(), "")
	return fr.NewValue(result, types.Typ[types.Bool])
}

func (fr *frame) makeInterface(v *LLVMValue, iface types.Type) *LLVMValue {
	llv := v.LLVMValue()
	i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
	if _, ok := v.Type().Underlying().(*types.Pointer); !ok {
		ptr := fr.createTypeMalloc(v.Type())
		fr.builder.CreateStore(llv, ptr)
		llv = fr.builder.CreateBitCast(ptr, i8ptr, "")
	}
	value := llvm.Undef(fr.types.ToLLVM(iface))
	itab := fr.types.getItabPointer(v.Type(), iface.Underlying().(*types.Interface))
	value = fr.builder.CreateInsertValue(value, itab, 0, "")
	value = fr.builder.CreateInsertValue(value, llv, 1, "")
	return fr.NewValue(value, iface)
}

// Reads the type descriptor from the given interface type.
func (fr *frame) getInterfaceTypeDescriptor(v *LLVMValue) llvm.Value {
	isempty := v.Type().Underlying().(*types.Interface).NumMethods() == 0
	itab := fr.builder.CreateExtractValue(v.LLVMValue(), 0, "")
	if isempty {
		return itab
	} else {
		itabnonnull := fr.builder.CreateIsNotNull(itab, "")
		return fr.loadOrNull(itabnonnull, itab, types.Typ[types.UnsafePointer]).LLVMValue()
	}
}

// Reads the value from the given interface type, assuming that the
// interface holds a value of the correct type.
func (fr *frame) getInterfaceValue(v *LLVMValue, ty types.Type) *LLVMValue {
	val := fr.builder.CreateExtractValue(v.LLVMValue(), 1, "")
	if _, ok := ty.Underlying().(*types.Pointer); !ok {
		typedval := fr.builder.CreateBitCast(val, llvm.PointerType(fr.types.ToLLVM(ty), 0), "")
		val = fr.builder.CreateLoad(typedval, "")
	}
	return fr.NewValue(val, ty)
}

// If cond is true, reads the value from the given interface type, otherwise
// returns a nil value.
func (fr *frame) getInterfaceValueOrNull(cond llvm.Value, v *LLVMValue, ty types.Type) *LLVMValue {
	val := fr.builder.CreateExtractValue(v.LLVMValue(), 1, "")
	if _, ok := ty.Underlying().(*types.Pointer); ok {
		val = fr.builder.CreateSelect(cond, val, llvm.ConstNull(val.Type()), "")
	} else {
		val = fr.loadOrNull(cond, val, ty).LLVMValue()
	}
	return fr.NewValue(val, ty)
}

func (fr *frame) interfaceTypeCheck(val *LLVMValue, ty types.Type) (v *LLVMValue, okval *LLVMValue) {
	tytd := fr.types.ToRuntime(ty)
	if _, ok := ty.Underlying().(*types.Interface); ok {
		var result []llvm.Value
		if val.Type().Underlying().(*types.Interface).NumMethods() > 0 {
			result = fr.runtime.ifaceI2I2.call(fr, tytd, val.LLVMValue())
		} else {
			result = fr.runtime.ifaceE2I2.call(fr, tytd, val.LLVMValue())
		}
		v = fr.NewValue(result[0], ty)
		okval = fr.NewValue(result[1], types.Typ[types.Bool])
	} else {
		valtd := fr.getInterfaceTypeDescriptor(val)
		tyequal := fr.runtime.typeDescriptorsEqual.call(fr, valtd, tytd)[0]
		okval = fr.NewValue(tyequal, types.Typ[types.Bool])
		tyequal = fr.builder.CreateTrunc(tyequal, llvm.Int1Type(), "")

		v = fr.getInterfaceValueOrNull(tyequal, val, ty)
	}
	return
}

func (fr *frame) interfaceTypeAssert(val *LLVMValue, ty types.Type) *LLVMValue {
	if _, ok := ty.Underlying().(*types.Interface); ok {
		return fr.changeInterface(val, ty, true)
	} else {
		valtytd := fr.types.ToRuntime(val.Type())
		valtd := fr.getInterfaceTypeDescriptor(val)
		tytd := fr.types.ToRuntime(ty)
		fr.runtime.checkInterfaceType.call(fr, valtd, tytd, valtytd)

		return fr.getInterfaceValue(val, ty)
	}
}

// convertI2E converts a non-empty interface value to an empty interface.
func (fr *frame) convertI2E(v *LLVMValue) *LLVMValue {
	td := fr.getInterfaceTypeDescriptor(v)
	val := fr.builder.CreateExtractValue(v.LLVMValue(), 1, "")

	typ := types.NewInterface(nil, nil)
	intf := llvm.Undef(fr.types.ToLLVM(typ))
	intf = fr.builder.CreateInsertValue(intf, td, 0, "")
	intf = fr.builder.CreateInsertValue(intf, val, 1, "")
	return fr.NewValue(intf, typ)
}

func (fr *frame) changeInterface(v *LLVMValue, ty types.Type, assert bool) *LLVMValue {
	td := fr.getInterfaceTypeDescriptor(v)
	tytd := fr.types.ToRuntime(ty)
	var itab llvm.Value
	if assert {
		itab = fr.runtime.assertInterface.call(fr, tytd, td)[0]
	} else {
		itab = fr.runtime.convertInterface.call(fr, tytd, td)[0]
	}
	val := fr.builder.CreateExtractValue(v.LLVMValue(), 1, "")

	intf := llvm.Undef(fr.types.ToLLVM(ty))
	intf = fr.builder.CreateInsertValue(intf, itab, 0, "")
	intf = fr.builder.CreateInsertValue(intf, val, 1, "")
	return fr.NewValue(intf, ty)
}
