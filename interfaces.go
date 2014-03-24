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
		a = a.convertI2E()
	case bI:
		b = b.convertI2E()
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
	rtype := fr.types.ToRuntime(v.Type())
	rtype = fr.builder.CreateBitCast(rtype, llvm.PointerType(llvm.Int8Type(), 0), "")
	value = fr.builder.CreateInsertValue(value, rtype, 0, "")
	value = fr.builder.CreateInsertValue(value, llv, 1, "")
	if iface.Underlying().(*types.Interface).NumMethods() > 0 {
		result := fr.NewValue(value, types.NewInterface(nil, nil))
		result, _ = result.convertE2I(iface)
		return result
	}
	return fr.NewValue(value, iface)
}
