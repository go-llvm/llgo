// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
	"go/ast"
)

func (c *compiler) VisitCap(expr *ast.CallExpr) Value {
	value := c.VisitExpr(expr.Args[0])
	typ := value.Type()
	if name, ok := typ.Underlying().(*types.Named); ok {
		typ = name.Underlying()
	}
	var capvalue llvm.Value
	switch typ := typ.Underlying().(type) {
	case *types.Pointer:
		atyp := typ.Elem().Underlying().(*types.Array)
		capvalue = llvm.ConstInt(c.llvmtypes.inttype, uint64(atyp.Len()), false)
	case *types.Array:
		capvalue = llvm.ConstInt(c.llvmtypes.inttype, uint64(typ.Len()), false)
	case *types.Slice:
		sliceval := value.LLVMValue()
		capvalue = c.builder.CreateExtractValue(sliceval, 2, "")
	case *types.Chan:
		panic("cap(chan) unimplemented")
	}
	return c.NewValue(capvalue, types.Typ[types.Int])
}

func (c *compiler) VisitLen(expr *ast.CallExpr) Value {
	value := c.VisitExpr(expr.Args[0])
	typ := value.Type()
	if name, ok := typ.Underlying().(*types.Named); ok {
		typ = name.Underlying()
	}
	var lenvalue llvm.Value
	switch typ := typ.Underlying().(type) {
	case *types.Pointer:
		atyp := typ.Elem().Underlying().(*types.Array)
		lenvalue = llvm.ConstInt(c.llvmtypes.inttype, uint64(atyp.Len()), false)
	case *types.Slice:
		sliceval := value.LLVMValue()
		lenvalue = c.builder.CreateExtractValue(sliceval, 1, "")
	case *types.Map:
		mapval := value.LLVMValue()
		f := c.NamedFunction("runtime.maplen", "func(m uintptr) int")
		lenvalue = c.builder.CreateCall(f, []llvm.Value{mapval}, "")
	case *types.Array:
		lenvalue = llvm.ConstInt(c.llvmtypes.inttype, uint64(typ.Len()), false)
	case *types.Basic:
		if isString(typ) {
			value := value.(*LLVMValue)
			lenvalue = c.builder.CreateExtractValue(value.LLVMValue(), 1, "")
		}
	case *types.Chan:
		chanval := value.LLVMValue()
		f := c.NamedFunction("runtime.chanlen", "func(c uintptr) int")
		lenvalue = c.builder.CreateCall(f, []llvm.Value{chanval}, "")
	}
	return c.NewValue(lenvalue, types.Typ[types.Int])
}

// vim: set ft=go :
