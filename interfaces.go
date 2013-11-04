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
	llvalue := c.builder.CreateExtractValue(lliface, 1, "")
	// TODO represent iface properly, as {*itab, value},
	// and extract interface pointer here.
	//llitab := ll

	// Strip receiver.
	sig := method.Type().(*types.Signature)
	sig = types.NewSignature(nil, nil, sig.Params(), sig.Results(), sig.IsVariadic())

	llfn := llvm.ConstNull(c.types.ToLLVM(sig))
	llfn = c.builder.CreateInsertValue(llfn, llvalue, 1, "")
	return c.NewValue(llfn, sig)
}

// compareInterfaces emits code to compare two interfaces for
// equality.
func (c *compiler) compareInterfaces(a, b *LLVMValue) *LLVMValue {
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
	f := c.runtime.compareE2E.LLVMValue()
	args := []llvm.Value{
		c.coerce(a.LLVMValue(), c.runtime.eface.llvm),
		c.coerce(b.LLVMValue(), c.runtime.eface.llvm),
	}
	return c.NewValue(c.builder.CreateCall(f, args, ""), types.Typ[types.Bool])
}

func (c *compiler) makeInterface(v *LLVMValue, iface types.Type) *LLVMValue {
	value := llvm.Undef(c.types.ToLLVM(iface))
	rtype := c.types.ToRuntime(v.Type())
	rtype = c.builder.CreateBitCast(rtype, llvm.PointerType(llvm.Int8Type(), 0), "")
	value = c.builder.CreateInsertValue(value, rtype, 0, "")
	value = c.builder.CreateInsertValue(value, v.interfaceValue(), 1, "")
	// TODO convE2I
	return c.NewValue(value, iface)
}
