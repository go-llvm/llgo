// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"fmt"
	"github.com/axw/gollvm/llvm"
	"go/ast"
	"go/types"
)

func (c *compiler) VisitCap(expr *ast.CallExpr) Value {
	// TODO implement me
	return c.VisitLen(expr)
}

func (c *compiler) VisitLen(expr *ast.CallExpr) Value {
	if len(expr.Args) > 1 {
		panic("Expecting only one argument to len")
	}

	value := c.VisitExpr(expr.Args[0])
	typ := value.Type()
	if name, ok := underlyingType(typ).(*types.NamedType); ok {
		typ = name.Underlying
	}

	var lenvalue llvm.Value
	switch typ := underlyingType(typ).(type) {
	case *types.Pointer:
		atyp := underlyingType(typ.Base).(*types.Array)
		lenvalue = llvm.ConstInt(c.llvmtypes.inttype, uint64(atyp.Len), false)

	case *types.Slice:
		sliceval := value.LLVMValue()
		lenvalue = c.builder.CreateExtractValue(sliceval, 1, "")

	case *types.Map:
		mapval := value.LLVMValue()
		f := c.NamedFunction("runtime.maplen", "func f(m uintptr) int")
		lenvalue = c.builder.CreateCall(f, []llvm.Value{mapval}, "")

	case *types.Array:
		lenvalue = llvm.ConstInt(c.llvmtypes.inttype, uint64(typ.Len), false)

	case *types.Basic:
		if isString(typ) {
			value := value.(*LLVMValue)
			lenvalue = c.builder.CreateExtractValue(value.LLVMValue(), 1, "")
		}
	}
	if !lenvalue.IsNil() {
		return c.NewValue(lenvalue, types.Typ[types.Int])
	}
	panic(fmt.Sprint("Unhandled value type: ", value.Type()))
}

// vim: set ft=go :
