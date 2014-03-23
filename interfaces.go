// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// interfaceMethod returns a function pointer for the specified
// interface and method pair.
func (c *compiler) interfaceMethod(iface *LLVMValue, method *types.Func) *LLVMValue {
	lliface := iface.LLVMValue()
	llitab := c.builder.CreateExtractValue(lliface, 0, "")
	llvalue := c.builder.CreateExtractValue(lliface, 1, "")
	sig := method.Type().(*types.Signature)
	methodset := c.types.MethodSet(sig.Recv().Type())
	// TODO(axw) cache ordered method index
	var index int
	for i := 0; i < methodset.Len(); i++ {
		if methodset.At(i).Obj() == method {
			index = i
			break
		}
	}
	llitab = c.builder.CreateBitCast(llitab, llvm.PointerType(c.runtime.itab.llvm, 0), "")
	llifn := c.builder.CreateGEP(llitab, []llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), 0, false),
		llvm.ConstInt(llvm.Int32Type(), 5, false), // index of itab.fun
	}, "")
	_ = index
	llifn = c.builder.CreateGEP(llifn, []llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), uint64(index), false),
	}, "")
	llifn = c.builder.CreateLoad(llifn, "")
	// Strip receiver.
	sig = types.NewSignature(nil, nil, sig.Params(), sig.Results(), sig.Variadic())
	llfn := llvm.Undef(c.types.ToLLVM(sig))
	llifn = c.builder.CreateIntToPtr(llifn, llfn.Type().StructElementTypes()[0], "")
	llfn = c.builder.CreateInsertValue(llfn, llifn, 0, "")
	llfn = c.builder.CreateInsertValue(llfn, llvalue, 1, "")
	return c.NewValue(llfn, sig)
}

// compareInterfaces emits code to compare two interfaces for
// equality.
func (c *compiler) compareInterfaces(a, b *LLVMValue) *LLVMValue {
	aNull := a.LLVMValue().IsNull()
	bNull := b.LLVMValue().IsNull()
	if aNull && bNull {
		return c.NewValue(boolLLVMValue(true), types.Typ[types.Bool])
	}
	if !aNull && !bNull {
		aI := a.Type().Underlying().(*types.Interface).NumMethods() > 0
		bI := b.Type().Underlying().(*types.Interface).NumMethods() > 0
		switch {
		case aI && bI:
			a = a.convertI2E()
			b = b.convertI2E()
		case aI:
			a = a.convertI2E()
		case bI:
			b = b.convertI2E()
		}
	}
	f := c.runtime.compareE2E.LLVMValue()
	args := []llvm.Value{
		coerce(c.builder, a.LLVMValue(), c.runtime.eface.llvm),
		coerce(c.builder, b.LLVMValue(), c.runtime.eface.llvm),
	}
	return c.NewValue(c.builder.CreateCall(f, args, ""), types.Typ[types.Bool])
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
