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
			curfunc := i.functions.Top()
			// FIXME this needs review; this will currently
			// pick up globals.
			if value.stack != curfunc && value.pointer != nil {
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
	/*
		ftyp := c.types.expr[lit].(*types.Signature)

		// Walk the function literal, promoting stack vars not defined
		// in the function literal, and storing the ident's for non-const
		// values not declared in the function literal.
		//
		// (First, set a dummy "stack" value for the params and results.)
		var dummyfunc LLVMValue
		dummyfunc.stack = &dummyfunc
		for _, obj := range ftyp.Params {
			obj.Data = dummyfunc
		}
		for _, obj := range ftyp.Results {
			obj.Data = dummyfunc
		}
		c.functions.Push(&dummyfunc)
		v := &identVisitor{compiler: c}
		ast.Walk(v, lit.Body)
		c.functions.Pop()

		// Create closure by adding a "next" parameter to the function,
		// and bind it with the values of the stack vars found in the
		// step above.
		//
		// First, we store the existing values, and replace them with
		// temporary values. Once we've created the function, we'll
		// replace the temporary values with pointers from the nest
		// parameter.
		var origValues []*LLVMValue
		if v.objects != nil {
			origValues = make([]*LLVMValue, len(v.objects))
			ptrObjects := make(types.ObjList, len(v.objects))
			for i, obj := range v.objects {
				origValues[i] = obj.Data.(*LLVMValue)
				ptrObjects[i] = ast.NewObj(ast.Var, "")
				ptrObjects[i].Type = &types.Pointer{Base: obj.Type.(types.Type)}
			}
			defer func() {
				for i, obj := range v.objects {
					obj.Data = origValues[i]
				}
			}()

			// Add the nest param.
			nestType := &types.Pointer{Base: &types.Struct{Fields: ptrObjects}}
			nestParamObj := ast.NewObj(ast.Var, "")
			nestParamObj.Type = nestType
			ftyp.Params = append(types.ObjList{nestParamObj}, ftyp.Params...)
			for _, obj := range v.objects {
				ptrType := &types.Pointer{Base: obj.Type.(types.Type)}
				ptrVal := c.builder.CreateAlloca(c.types.ToLLVM(ptrType.Base), "")
				obj.Data = c.NewLLVMValue(ptrVal, ptrType).makePointee()
			}
		}

		fn_value := llvm.AddFunction(c.module.Module, "", c.types.ToLLVM(ftyp).ElementType())
		fn_value.SetFunctionCallConv(llvm.FastCallConv)
		currBlock := c.builder.GetInsertBlock()
		f := c.NewLLVMValue(fn_value, ftyp)
		c.buildFunction(f, ftyp.Params, lit.Body)

		// Closure? Erect a trampoline.
		if v.objects != nil {
			blockPtr := fn_value.FirstParam()
			blockPtr.AddAttribute(llvm.NestAttribute)
			ftyp.Params = ftyp.Params[1:]
			paramTypes := fn_value.Type().ElementType().ParamTypes()
			c.builder.SetInsertPointBefore(fn_value.EntryBasicBlock().FirstInstruction())
			for i, obj := range v.objects {
				tempPtrVal := obj.Data.(*LLVMValue).pointer.value
				argPtrVal := c.builder.CreateStructGEP(blockPtr, i, "")
				tempPtrVal.ReplaceAllUsesWith(c.builder.CreateLoad(argPtrVal, ""))
			}
			c.builder.SetInsertPointAtEnd(currBlock)

			// FIXME This is only correct for x86. Not sure what the best
			// thing to do is here; should we even create trampolines?
			// Or is it better to pass a (block, funcptr) pair around?
			memalign := c.NamedFunction("runtime.memalign", "func f(align, size uintptr) *int8")
			align := uint64(c.target.PointerSize())
			args := []llvm.Value{
				llvm.ConstInt(c.target.IntPtrType(), align, false), // alignment
				llvm.ConstInt(c.target.IntPtrType(), 100, false),   // size of trampoline
			}
			tramp := c.builder.CreateCall(memalign, args, "tramp")

			// Store the free variables in the heap allocated block.
			block := c.createTypeMalloc(paramTypes[0].ElementType())
			for i, _ := range v.objects {
				ptrVal := origValues[i].pointer.LLVMValue()
				blockPtr := c.builder.CreateStructGEP(block, i, "")
				c.builder.CreateStore(ptrVal, blockPtr)
			}

			initTrampFunc := c.NamedFunction("llvm.init.trampoline", "func f(tramp, fun, nval *int8)")
			i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
			funcptr := c.builder.CreateBitCast(fn_value, i8ptr, "")
			block = c.builder.CreateBitCast(block, i8ptr, "")
			c.builder.CreateCall(initTrampFunc, []llvm.Value{tramp, funcptr, block}, "")
			adjustTrampFunc := c.NamedFunction("llvm.adjust.trampoline", "func f(tramp *int8) *int8")
			funcptr = c.builder.CreateCall(adjustTrampFunc, []llvm.Value{tramp}, "")
			f.value = c.builder.CreateBitCast(funcptr, c.types.ToLLVM(ftyp), "")
		} else {
			c.builder.SetInsertPointAtEnd(currBlock)
		}

		return f
	*/
	panic("unimplemented: function literals")
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
					typ := underlyingType(typ).(*types.Array)
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
		// TODO check result of mallocs
		length := llvm.ConstInt(llvm.Int32Type(), uint64(len(valuelist)), false)
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
