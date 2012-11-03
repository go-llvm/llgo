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
	v := c.NewConstValue(lit.Kind, lit.Value)
	if typ, ok := c.types.expr[lit]; ok {
		v.typ = typ
		v.Const = v.Const.Convert(&typ)
	}
	return v
}

func (c *compiler) VisitFuncLit(lit *ast.FuncLit) Value {
	ftyp := c.types.expr[lit].(*types.Func)
	fn_value := llvm.AddFunction(c.module.Module, "", c.types.ToLLVM(ftyp).ElementType())
	fn_value.SetFunctionCallConv(llvm.FastCallConv)
	defer c.builder.SetInsertPointAtEnd(c.builder.GetInsertBlock())
	f := c.NewLLVMValue(fn_value, ftyp)
	c.buildFunction(f, ftyp.Params, lit.Body)
	return f
}

func (c *compiler) VisitCompositeLit(lit *ast.CompositeLit) Value {
	typ := c.types.expr[lit]
	var valuemap map[interface{}]Value
	var valuelist []Value
	_, isstruct := types.Underlying(typ).(*types.Struct)
	if lit.Elts != nil {
		for _, elt := range lit.Elts {
			var value Value
			if kv, iskv := elt.(*ast.KeyValueExpr); iskv {
				value = c.VisitExpr(kv.Value)
				if valuemap == nil {
					valuemap = make(map[interface{}]Value)
				}
				var key interface{}
				if isstruct {
					key = kv.Key.(*ast.Ident).Name
				} else {
					key = c.VisitExpr(kv.Key)
				}
				valuemap[key] = value
			} else {
				value = c.VisitExpr(elt)
				valuelist = append(valuelist, value)
			}
		}
	}

	// For array/slice types, convert key:value to contiguous
	// values initialiser.
	switch types.Underlying(typ).(type) {
	case *types.Array, *types.Slice:
		if len(valuemap) > 0 {
			maxi := int64(-1)
			for key, _ := range valuemap {
				i := key.(ConstValue).Int64()
				if i < 0 {
					panic("array index must be non-negative integer constant")
				} else if i > maxi {
					maxi = i
				}
			}
			valuelist = make([]Value, maxi+1)
			for key, value := range valuemap {
				i := key.(ConstValue).Int64()
				valuelist[i] = value
			}
		}
	}

	origtyp := typ
	switch typ := types.Underlying(typ).(type) {
	case *types.Array:
		typ.Len = uint64(len(valuelist))
		elttype := typ.Elt
		llvm_values := make([]llvm.Value, typ.Len)
		for i, value := range valuelist {
			if value == nil {
				llvm_values[i] = llvm.ConstNull(c.types.ToLLVM(elttype))
			} else {
				llvm_values[i] = value.Convert(elttype).LLVMValue()
			}
		}
		// TODO set non-const values after creating const array.
		return c.NewLLVMValue(
			llvm.ConstArray(c.types.ToLLVM(elttype), llvm_values), origtyp)

	case *types.Slice:
		ptr := c.builder.CreateMalloc(c.types.ToLLVM(typ), "")
		length := llvm.ConstInt(llvm.Int32Type(), uint64(len(valuelist)), false)
		valuesPtr := c.builder.CreateArrayMalloc(c.types.ToLLVM(typ.Elt), length, "")
		//valuesPtr = c.builder.CreateBitCast(valuesPtr, llvm.PointerType(valuesPtr.Type(), 0), "")
		// TODO check result of mallocs
		c.builder.CreateStore(valuesPtr, c.builder.CreateStructGEP(ptr, 0, "")) // data
		c.builder.CreateStore(length, c.builder.CreateStructGEP(ptr, 1, ""))    // len
		c.builder.CreateStore(length, c.builder.CreateStructGEP(ptr, 2, ""))    // cap
		null := llvm.ConstNull(c.types.ToLLVM(typ.Elt))
		for i, value := range valuelist {
			index := llvm.ConstInt(llvm.Int32Type(), uint64(i), false)
			valuePtr := c.builder.CreateGEP(valuesPtr, []llvm.Value{index}, "")
			if value == nil {
				c.builder.CreateStore(null, valuePtr)
			} else {
				c.builder.CreateStore(value.Convert(typ.Elt).LLVMValue(), valuePtr)
			}
		}
		m := c.NewLLVMValue(ptr, &types.Pointer{Base: origtyp})
		return m.makePointee()

	case *types.Struct:
		values := valuelist
		struct_value := c.builder.CreateMalloc(c.types.ToLLVM(typ), "")
		if valuemap != nil {
			for key, value := range valuemap {
				fieldName := key.(string)
				index := typ.FieldIndices[fieldName]
				for len(values) <= int(index) {
					values = append(values, nil)
				}
				values[index] = value
			}
		}
		for i, value := range values {
			elttype := typ.Fields[i].Type.(types.Type)
			var llvm_value llvm.Value
			if value == nil {
				llvm_value = llvm.ConstNull(c.types.ToLLVM(elttype))
			} else {
				llvm_value = value.Convert(elttype).LLVMValue()
			}
			ptr := c.builder.CreateStructGEP(struct_value, i, "")
			c.builder.CreateStore(llvm_value, ptr)
		}
		m := c.NewLLVMValue(struct_value, &types.Pointer{Base: origtyp})
		return m.makePointee()

	case *types.Map:
		value := llvm.ConstNull(c.types.ToLLVM(typ))
		// TODO initialise map
		return c.NewLLVMValue(value, origtyp)
	}
	panic(fmt.Sprint("Unhandled type kind: ", typ))
}

// vim: set ft=go :
