/*
Copyright (c) 2011, 2012 Andrew Wilkins <axwalk@gmail.com>

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
	"fmt"
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo/types"
	"go/ast"
)

func (c *compiler) VisitBasicLit(lit *ast.BasicLit) Value {
	return c.NewConstValue(lit.Kind, lit.Value)
}

func (c *compiler) VisitFuncLit(lit *ast.FuncLit) Value {
	fn_type := c.VisitFuncType(lit.Type)
	fn := llvm.AddFunction(c.module.Module, "", c.types.ToLLVM(fn_type))
	fn.SetFunctionCallConv(llvm.FastCallConv)

	defer c.builder.SetInsertPointAtEnd(c.builder.GetInsertBlock())
	entry := llvm.AddBasicBlock(fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)

	fn_value := c.NewLLVMValue(fn, fn_type)
	c.functions = append(c.functions, fn_value)
	c.VisitBlockStmt(lit.Body)
	if fn_type.Results == nil {
		lasti := entry.LastInstruction()
		if lasti.IsNil() || lasti.Opcode() != llvm.Ret {
			// Assume nil return type, AST should be checked first.
			c.builder.CreateRetVoid()
		}
	}
	c.functions = c.functions[0 : len(c.functions)-1]
	return fn_value
}

func (c *compiler) VisitCompositeLit(lit *ast.CompositeLit) Value {
	typ := c.GetType(lit.Type)
	var values []Value
	if lit.Elts != nil {
		// TODO handle string keys.
		valuemap := make(map[int]Value)
		maxi := 0
		for i, elt := range lit.Elts {
			var value Value
			if kv, iskv := elt.(*ast.KeyValueExpr); iskv {
				key := c.VisitExpr(kv.Key)
				i = -1
				if const_key, isconst := key.(ConstValue); isconst {
					i = int(const_key.Int64())
				}
				value = c.VisitExpr(kv.Value)
			} else {
				value = c.VisitExpr(elt)
			}
			if i >= 0 {
				if i > maxi {
					maxi = i
				}
				valuemap[i] = value
			} else {
				panic("array index must be non-negative integer constant")
			}
		}
		values = make([]Value, maxi+1)
		for i, value := range valuemap {
			values[i] = value
		}
	}

	origtyp := typ
	switch typ := types.Underlying(typ).(type) {
	case *types.Array:
		typ.Len = uint64(len(values))
		elttype := typ.Elt
		llvm_values := make([]llvm.Value, len(values))
		for i, value := range values {
			if value == nil {
				llvm_values[i] = llvm.ConstNull(c.types.ToLLVM(elttype))
			} else {
				if lv, islv := value.(*LLVMValue); islv && lv.indirect {
					value = lv.Deref()
				}
				llvm_values[i] = value.Convert(elttype).LLVMValue()
			}
		}
		// TODO set non-const values after creating const array.
		return c.NewLLVMValue(
			llvm.ConstArray(c.types.ToLLVM(elttype), llvm_values), origtyp)

	case *types.Struct:
		struct_value := c.builder.CreateMalloc(c.types.ToLLVM(typ), "")
		for i, value := range values {
			elttype := c.ObjGetType(typ.Fields[i])
			var llvm_value llvm.Value
			if value == nil {
				llvm_value = llvm.ConstNull(c.types.ToLLVM(elttype))
			} else {
				if lv, islv := value.(*LLVMValue); islv && lv.indirect {
					value = lv.Deref()
				}
				llvm_value = value.Convert(elttype).LLVMValue()
			}
			ptr := c.builder.CreateStructGEP(struct_value, i, "")
			c.builder.CreateStore(llvm_value, ptr)
		}
		m := c.NewLLVMValue(struct_value, &types.Pointer{Base: origtyp})
		m.indirect = true
		return m
	}
	panic(fmt.Sprint("Unhandled type kind: ", typ))
}

// vim: set ft=go :
