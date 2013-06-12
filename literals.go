// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/exact"
	"code.google.com/p/go.tools/go/types"
	"fmt"
	"github.com/greggoryhz/gollvm/llvm"
	"go/ast"
	"go/token"
)

type identVisitor struct {
	*compiler
	captures []*types.Var
}

func (i *identVisitor) Visit(node ast.Node) ast.Visitor {
	switch node := node.(type) {
	case *ast.Ident:
		if v, ok := i.objects[node].(*types.Var); ok {
			value := i.objectdata[v].Value
			curfunc := i.functions.top()
			// FIXME this needs review; this will currently
			// pick up globals.
			if value != nil && value.stack != curfunc.LLVMValue && value.pointer != nil {
				if value.stack != nil {
					value.promoteStackVar()
				}
				// We're referring to a variable that was
				// not declared on the current function's
				// stack.
				i.captures = append(i.captures, v)
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
	paramVars := ftyp.Params()
	resultVars := ftyp.Results()
	c.functions.push(&function{
		LLVMValue: &dummyfunc,
		results:   resultVars,
	})
	v := &identVisitor{compiler: c}
	ast.Walk(v, lit.Body)
	c.functions.pop()

	// Create closure by adding a context parameter to the function,
	// and bind it with the values of the stack vars found in the
	// step above.
	origfnpairtyp := c.types.ToLLVM(ftyp)
	fnpairtyp := origfnpairtyp
	fntyp := origfnpairtyp.StructElementTypes()[0].ElementType()
	if v.captures != nil {
		// Add the additional context param.
		ctxfields := make([]*types.Field, len(v.captures))
		for i, capturevar := range v.captures {
			p := capturevar.Pkg()
			n := capturevar.Name()
			t := types.NewPointer(capturevar.Type())
			ctxfields[i] = types.NewField(token.NoPos, p, n, t, false)
		}
		ctxtyp := types.NewPointer(types.NewStruct(ctxfields, nil))
		llvmctxtyp := c.types.ToLLVM(ctxtyp)
		rettyp := fntyp.ReturnType()
		paramtyps := append([]llvm.Type{llvmctxtyp}, fntyp.ParamTypes()...)
		vararg := fntyp.IsFunctionVarArg()
		fntyp = llvm.FunctionType(rettyp, paramtyps, vararg)
		opaqueptrtyp := origfnpairtyp.StructElementTypes()[1]
		elttyps := []llvm.Type{llvm.PointerType(fntyp, 0), opaqueptrtyp}
		fnpairtyp = llvm.StructType(elttyps, false)
	}

	fnptr := llvm.AddFunction(c.module.Module, "", fntyp)
	fnvalue := llvm.ConstNull(fnpairtyp)
	fnvalue = llvm.ConstInsertValue(fnvalue, fnptr, []uint32{0})
	currBlock := c.builder.GetInsertBlock()

	f := c.NewValue(fnvalue, ftyp)
	captureVars := types.NewTuple(v.captures...)
	c.buildFunction(f, captureVars, paramVars, resultVars, lit.Body, ftyp.IsVariadic())

	// Closure? Bind values to a context block.
	if v.captures != nil {
		// Store the free variables in the heap allocated block.
		block := c.createTypeMalloc(fntyp.ParamTypes()[0].ElementType())
		for i, contextvar := range v.captures {
			value := c.objectdata[contextvar].Value
			blockPtr := c.builder.CreateStructGEP(block, i, "")
			c.builder.CreateStore(value.pointer.LLVMValue(), blockPtr)
		}

		// Cast the function pointer type back to the original
		// type, without the context parameter.
		fnptr = llvm.ConstBitCast(fnptr, origfnpairtyp.StructElementTypes()[0])
		fnvalue = llvm.Undef(origfnpairtyp)
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

func (c *compiler) VisitCompositeLit(lit *ast.CompositeLit) (v *LLVMValue) {
	typ := c.types.expr[lit].Type
	var valuemap map[interface{}]Value
	var valuelist []Value

	if ptr, ok := typ.(*types.Pointer); ok {
		typ = ptr.Elem()
		defer func() {
			v = v.pointer
		}()
	}

	var isstruct, isarray, isslice, ismap bool
	switch typ.Underlying().(type) {
	case *types.Struct:
		isstruct = true
	case *types.Array:
		isarray = true
	case *types.Slice:
		isslice = true
	case *types.Map:
		ismap = true
	default:
		panic(fmt.Errorf("Unhandled type: %s", typ))
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
					typ := typ.Underlying().(*types.Struct)
					elttyp = typ.Field(fieldIndex(typ, name)).Type()
				case isarray:
					key = c.types.expr[kv.Key].Value
					typ := typ.Underlying().(*types.Array)
					elttyp = typ.Elem()
				case isslice:
					key = c.types.expr[kv.Key].Value
					typ := typ.Underlying().(*types.Slice)
					elttyp = typ.Elem()
				case ismap:
					key = c.VisitExpr(kv.Key)
					typ := typ.Underlying().(*types.Map)
					elttyp = typ.Elem()
				default:
					panic("unreachable")
				}
				c.convertUntyped(kv.Value, elttyp)
				valuemap[key] = c.VisitExpr(kv.Value)
			} else {
				switch {
				case isstruct:
					typ := typ.Underlying().(*types.Struct)
					c.convertUntyped(elt, typ.Field(i).Type())
				case isarray:
					typ := typ.Underlying().(*types.Array)
					c.convertUntyped(elt, typ.Elem())
				case isslice:
					typ := typ.Underlying().(*types.Slice)
					c.convertUntyped(elt, typ.Elem())
				}
				value := c.VisitExpr(elt)
				valuelist = append(valuelist, value)
			}
		}
	}

	// For array/slice types, convert key:value to contiguous
	// values initialiser.
	switch typ.Underlying().(type) {
	case *types.Array, *types.Slice:
		if len(valuemap) > 0 {
			var maxkey uint64
			for key, _ := range valuemap {
				key, _ := exact.Uint64Val(key.(exact.Value))
				if key > maxkey {
					maxkey = key
				}
			}
			valuelist = make([]Value, maxkey+1)
			for key, value := range valuemap {
				key, _ := exact.Uint64Val(key.(exact.Value))
				valuelist[key] = value
			}
		}
	}

	origtyp := typ
	switch typ := typ.Underlying().(type) {
	case *types.Array:
		elttype := typ.Elem()
		llvmelttype := c.types.ToLLVM(elttype)
		llvmvalues := make([]llvm.Value, typ.Len())
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

		eltType := c.types.ToLLVM(typ.Elem())
		arrayType := llvm.ArrayType(eltType, len(valuelist))
		valuesPtr := c.createMalloc(llvm.SizeOf(arrayType))
		valuesPtr = c.builder.CreateIntToPtr(valuesPtr, llvm.PointerType(eltType, 0), "")

		//valuesPtr = c.builder.CreateBitCast(valuesPtr, llvm.PointerType(valuesPtr.Type(), 0), "")
		length := llvm.ConstInt(c.types.inttype, uint64(len(valuelist)), false)
		c.builder.CreateStore(valuesPtr, c.builder.CreateStructGEP(ptr, 0, "")) // data
		c.builder.CreateStore(length, c.builder.CreateStructGEP(ptr, 1, ""))    // len
		c.builder.CreateStore(length, c.builder.CreateStructGEP(ptr, 2, ""))    // cap
		null := llvm.ConstNull(c.types.ToLLVM(typ.Elem()))
		for i, value := range valuelist {
			index := llvm.ConstInt(llvm.Int32Type(), uint64(i), false)
			valuePtr := c.builder.CreateGEP(valuesPtr, []llvm.Value{index}, "")
			if value == nil {
				c.builder.CreateStore(null, valuePtr)
			} else {
				c.builder.CreateStore(value.Convert(typ.Elem()).LLVMValue(), valuePtr)
			}
		}
		m := c.NewValue(ptr, types.NewPointer(origtyp))
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
				elttype := typ.Field(i).Type()
				llvm_value := value.Convert(elttype).LLVMValue()
				ptr := c.builder.CreateStructGEP(ptr, i, "")
				c.builder.CreateStore(llvm_value, ptr)
			}
		}
		m := c.NewValue(ptr, types.NewPointer(origtyp))
		return m.makePointee()

	case *types.Map:
		value := llvm.ConstNull(c.types.ToLLVM(typ))
		// TODO initialise map
		return c.NewValue(value, origtyp)
	}
	panic(fmt.Sprint("Unhandled type kind: ", typ))
}
