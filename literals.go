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
	"go/ast"
	"go/types"
)

type identVisitor struct {
	*compiler
	objects []*ast.Object
}

func (i *identVisitor) Visit(node ast.Node) ast.Visitor {
	switch node := node.(type) {
	case *ast.Ident:
		if value, ok := node.Obj.Data.(*LLVMValue); ok {
			curfunc := i.functions.top()
			// FIXME this needs review; this will currently
			// pick up globals.
			if value.stack != curfunc.LLVMValue && value.pointer != nil {
				if value.stack != nil {
					value.promoteStackVar()
				}
				// We're referring to a variable that was
				// not declared on the current function's
				// stack.
				i.objects = append(i.objects, node.Obj)
			}
		}
	case *ast.FuncLit:
		// Don't recurse through multiple function literals.
		return nil
	}
	return i
}

func (c *compiler) VisitFuncLit(lit *ast.FuncLit) Value {
	ftyp := c.types.expr[lit].Type.(*types.Signature)

	// Walk the function literal, promoting stack vars not defined
	// in the function literal, and storing the ident's for non-const
	// values not declared in the function literal.
	//
	// (First, set a dummy "stack" value for the params and results.)
	var dummyfunc LLVMValue
	dummyfunc.stack = &dummyfunc
	paramObjs := fieldListObjects(lit.Type.Params)
	resultObjs := fieldListObjects(lit.Type.Results)
	/*
		for _, obj := range paramObjs {
			if obj != nil {
				obj.Data = dummyfunc
			}
		}
		for _, obj := range resultObjs {
			if obj != nil {
				obj.Data = dummyfunc
			}
		}
	*/
	c.functions.push(&function{
		LLVMValue: &dummyfunc,
		params:    paramObjs,
		results:   resultObjs,
	})
	v := &identVisitor{compiler: c}
	ast.Walk(v, lit.Body)
	c.functions.pop()

	// Create closure by adding a context parameter to the function,
	// and bind it with the values of the stack vars found in the
	// step above.
	//
	// First, we store the existing values, and replace them with
	// temporary values. Once we've created the function, we'll
	// replace the temporary values with pointers from the context
	// parameter.
	var origValues []*LLVMValue
	if v.objects != nil {
		origValues = make([]*LLVMValue, len(v.objects))
		ptrTypeFields := make([]*types.Field, len(v.objects))
		for i, obj := range v.objects {
			origValues[i] = obj.Data.(*LLVMValue)
			ptrTypeFields[i] = &types.Field{
				Type: &types.Pointer{Base: obj.Type.(types.Type)},
			}
		}
		defer func() {
			for i, obj := range v.objects {
				obj.Data = origValues[i]
			}
		}()

		// Add the additional context param.
		contextType := &types.Pointer{Base: &types.Struct{Fields: ptrTypeFields}}
		contextParam := &types.Var{Type: contextType}
		ftyp.Params = append([]*types.Var{contextParam}, ftyp.Params...)
		for _, obj := range v.objects {
			ptrType := &types.Pointer{Base: obj.Type.(types.Type)}
			ptrVal := c.builder.CreateAlloca(c.types.ToLLVM(ptrType.Base), "")
			obj.Data = c.NewValue(ptrVal, ptrType).makePointee()
		}
		contextParamObj := ast.NewObj(ast.Var, "")
		contextParamObj.Type = contextType
		paramObjs = append([]*ast.Object{contextParamObj}, paramObjs...)
	}

	llvmfntyp := c.types.ToLLVM(ftyp)
	fnptrtyp := llvmfntyp.StructElementTypes()[0].ElementType()
	fnptr := llvm.AddFunction(c.module.Module, "", fnptrtyp)
	fnvalue := llvm.ConstNull(llvmfntyp)
	fnvalue = llvm.ConstInsertValue(fnvalue, fnptr, []uint32{0})
	currBlock := c.builder.GetInsertBlock()

	f := c.NewValue(fnvalue, ftyp)
	c.buildFunction(f, paramObjs, resultObjs, lit.Body)

	// Closure? Bind values to a context block.
	if v.objects != nil {
		blockPtr := fnptr.FirstParam()
		ftyp.Params = ftyp.Params[1:]
		paramTypes := fnptr.Type().ElementType().ParamTypes()
		c.builder.SetInsertPointBefore(fnptr.EntryBasicBlock().FirstInstruction())
		for i, obj := range v.objects {
			tempPtrVal := obj.Data.(*LLVMValue).pointer.value
			argPtrVal := c.builder.CreateStructGEP(blockPtr, i, "")
			tempPtrVal.ReplaceAllUsesWith(c.builder.CreateLoad(argPtrVal, ""))
		}
		c.builder.SetInsertPointAtEnd(currBlock)

		// Store the free variables in the heap allocated block.
		block := c.createTypeMalloc(paramTypes[0].ElementType())
		for i, _ := range v.objects {
			ptrVal := origValues[i].pointer.LLVMValue()
			blockPtr := c.builder.CreateStructGEP(block, i, "")
			c.builder.CreateStore(ptrVal, blockPtr)
		}

		// Cast the function pointer type back to the original
		// type, without the context parameter.
		llvmfntyp := c.types.ToLLVM(ftyp)
		fnptr = llvm.ConstBitCast(fnptr, llvmfntyp.StructElementTypes()[0])
		fnvalue = llvm.Undef(llvmfntyp)
		fnvalue = llvm.ConstInsertValue(fnvalue, fnptr, []uint32{0})

		// Set the context value.
		i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
		block = c.builder.CreateBitCast(block, i8ptr, "")
		fnvalue = c.builder.CreateInsertValue(fnvalue, block, 1, "")
		f.value = fnvalue
	} else {
		c.builder.SetInsertPointAtEnd(currBlock)
	}

	return f
}

func (c *compiler) VisitCompositeLit(lit *ast.CompositeLit) Value {
	typ := c.types.expr[lit].Type
	var valuemap map[interface{}]Value
	var valuelist []Value

	var isstruct, isarray, isslice, ismap bool
	switch underlyingType(typ).(type) {
	case *types.Struct:
		isstruct = true
	case *types.Array:
		isarray = true
	case *types.Slice:
		isslice = true
	default:
		ismap = true
	}

	if lit.Elts != nil {
		for i, elt := range lit.Elts {
			if kv, iskv := elt.(*ast.KeyValueExpr); iskv {
				if valuemap == nil {
					valuemap = make(map[interface{}]Value)
				}
				var key interface{}
				var elttyp types.Type
				switch {
				case isstruct:
					name := kv.Key.(*ast.Ident).Name
					key = name
					typ := underlyingType(typ).(*types.Struct)
					elttyp = typ.Fields[fieldIndex(typ, name)].Type
				case isarray:
					key = c.types.expr[kv.Key].Value
					typ := underlyingType(typ).(*types.Array)
					elttyp = typ.Elt
				case isslice:
					key = c.types.expr[kv.Key].Value
					typ := underlyingType(typ).(*types.Slice)
					elttyp = typ.Elt
				case ismap:
					key = c.VisitExpr(kv.Key)
					typ := underlyingType(typ).(*types.Map)
					elttyp = typ.Elt
				default:
					panic("unreachable")
				}
				c.convertUntyped(kv.Value, elttyp)
				valuemap[key] = c.VisitExpr(kv.Value)
			} else {
				switch {
				case isstruct:
					typ := underlyingType(typ).(*types.Struct)
					c.convertUntyped(elt, typ.Fields[i].Type)
				case isarray:
					typ := underlyingType(typ).(*types.Array)
					c.convertUntyped(elt, typ.Elt)
				case isslice:
					typ := underlyingType(typ).(*types.Slice)
					c.convertUntyped(elt, typ.Elt)
				}
				value := c.VisitExpr(elt)
				valuelist = append(valuelist, value)
			}
		}
	}

	// For array/slice types, convert key:value to contiguous
	// values initialiser.
	switch underlyingType(typ).(type) {
	case *types.Array, *types.Slice:
		if len(valuemap) > 0 {
			maxi := int64(-1)
			for key, _ := range valuemap {
				i := key.(int64)
				if i < 0 {
					panic("array index must be non-negative integer constant")
				} else if i > maxi {
					maxi = i
				}
			}
			valuelist = make([]Value, maxi+1)
			for key, value := range valuemap {
				valuelist[key.(int64)] = value
			}
		}
	}

	origtyp := typ
	switch typ := underlyingType(typ).(type) {
	case *types.Array:
		elttype := typ.Elt
		llvmelttype := c.types.ToLLVM(elttype)
		llvmvalues := make([]llvm.Value, typ.Len)
		for i := range llvmvalues {
			var value Value
			if i < len(valuelist) {
				value = valuelist[i]
			}
			if value == nil {
				llvmvalues[i] = llvm.ConstNull(llvmelttype)
			} else if value.LLVMValue().IsConstant() {
				llvmvalues[i] = value.Convert(elttype).LLVMValue()
			} else {
				llvmvalues[i] = llvm.Undef(llvmelttype)
			}
		}
		array := llvm.ConstArray(llvmelttype, llvmvalues)
		for i, value := range valuelist {
			if llvmvalues[i].IsUndef() {
				value := value.Convert(elttype).LLVMValue()
				array = c.builder.CreateInsertValue(array, value, i, "")
			}
		}
		return c.NewValue(array, origtyp)

	case *types.Slice:
		ptr := c.createTypeMalloc(c.types.ToLLVM(typ))

		eltType := c.types.ToLLVM(typ.Elt)
		arrayType := llvm.ArrayType(eltType, len(valuelist))
		valuesPtr := c.createMalloc(llvm.SizeOf(arrayType))
		valuesPtr = c.builder.CreateIntToPtr(valuesPtr, llvm.PointerType(eltType, 0), "")

		//valuesPtr = c.builder.CreateBitCast(valuesPtr, llvm.PointerType(valuesPtr.Type(), 0), "")
		length := llvm.ConstInt(c.types.inttype, uint64(len(valuelist)), false)
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
		m := c.NewValue(ptr, &types.Pointer{Base: origtyp})
		return m.makePointee()

	case *types.Struct:
		values := valuelist
		llvmtyp := c.types.ToLLVM(typ)
		ptr := c.createTypeMalloc(llvmtyp)

		if valuemap != nil {
			for key, value := range valuemap {
				index := fieldIndex(typ, key.(string))
				for len(values) <= index {
					values = append(values, nil)
				}
				values[index] = value
			}
		}
		for i, value := range values {
			if value != nil {
				elttype := typ.Fields[i].Type.(types.Type)
				llvm_value := value.Convert(elttype).LLVMValue()
				ptr := c.builder.CreateStructGEP(ptr, i, "")
				c.builder.CreateStore(llvm_value, ptr)
			}
		}
		m := c.NewValue(ptr, &types.Pointer{Base: origtyp})
		return m.makePointee()

	case *types.Map:
		value := llvm.ConstNull(c.types.ToLLVM(typ))
		// TODO initialise map
		return c.NewValue(value, origtyp)
	}
	panic(fmt.Sprint("Unhandled type kind: ", typ))
}
