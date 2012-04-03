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
	"reflect"
	"sort"
)

func isglobal(value Value) bool {
	//return !value.IsAGlobalVariable().IsNil()
	return false
}

func (c *compiler) VisitBinaryExpr(expr *ast.BinaryExpr) Value {
	lhs := c.VisitExpr(expr.X)
	rhs := c.VisitExpr(expr.Y)
	return lhs.BinaryOp(expr.Op, rhs)
}

func (c *compiler) VisitUnaryExpr(expr *ast.UnaryExpr) Value {
	value := c.VisitExpr(expr.X)
	return value.UnaryOp(expr.Op)
}

func (c *compiler) VisitCallExpr(expr *ast.CallExpr) Value {
	var fn *LLVMValue
	switch x := (expr.Fun).(type) {
	case *ast.Ident:
		switch x.String() {
		case "println":
			return c.VisitPrintln(expr)
		case "len":
			return c.VisitLen(expr)
		case "new":
			return c.VisitNew(expr)
		default:
			// Is it a named type? Then this is a conversion (e.g. int(123))
			if len(expr.Args) == 1 {
				typ := c.GetType(x)
				if _, ok := typ.(*types.Name); ok {
					value := c.VisitExpr(expr.Args[0])
					return value.Convert(typ)
				}
			}

			fn = c.Resolve(x.Obj).(*LLVMValue)
			if fn == nil {
				panic(fmt.Sprintf(
					"No function found with name '%s'", x.String()))
			}
		}
	default:
		fn = c.VisitExpr(expr.Fun).(*LLVMValue)
	}
	if fn.indirect {
		fn = fn.Deref()
	}

	fn_type := fn.Type().(*types.Func)
	args := make([]llvm.Value, 0)
	if fn_type.Recv != nil {
		// Don't dereference the receiver here. It'll have been worked out in
		// the selector.
		receiver := fn.receiver
		args = append(args, receiver.LLVMValue())
	}
	if nparams := len(fn_type.Params); nparams > 0 {
		if fn_type.IsVariadic {
			nparams--
		}
		for i := 0; i < nparams; i++ {
			value := c.VisitExpr(expr.Args[i])
			if value_, isllvm := value.(*LLVMValue); isllvm {
				if value_.indirect {
					value = value_.Deref()
				}
			}
			param_type := fn_type.Params[i].Type.(types.Type)
			args = append(args, value.Convert(param_type).LLVMValue())
		}
		if fn_type.IsVariadic {
			param_type := fn_type.Params[nparams].Type.(types.Type)
			varargs := make([]llvm.Value, 0)
			for i := nparams; i < len(expr.Args); i++ {
				value := c.VisitExpr(expr.Args[i])
				if value_, isllvm := value.(*LLVMValue); isllvm {
					if value_.indirect {
						value = value_.Deref()
					}
				}
				value = value.Convert(param_type)
				varargs = append(varargs, value.LLVMValue())
			}
			slice_value := c.makeSlice(varargs, param_type)
			args = append(args, slice_value)
		}
	}

	var result_type types.Type
	switch len(fn_type.Results) {
	case 0: // no-op
	case 1:
		result_type = fn_type.Results[0].Type.(types.Type)
	default:
		fields := make([]*ast.Object, len(fn_type.Results))
		for i, result := range fn_type.Results {
			fields[i] = result
		}
		result_type = &types.Struct{Fields: fields}
	}

	return c.NewLLVMValue(
		c.builder.CreateCall(fn.LLVMValue(), args, ""),
		result_type)
}

func isIntType(t types.Type) bool {
	for {
		switch x := t.(type) {
		case *types.Name:
			t = x.Underlying
		case *types.Basic:
			switch x.Kind {
			case types.Uint8Kind:
				fallthrough
			case types.Uint16Kind:
				fallthrough
			case types.Uint32Kind:
				fallthrough
			case types.Uint64Kind:
				fallthrough
			case types.Int8Kind:
				fallthrough
			case types.Int16Kind:
				fallthrough
			case types.Int32Kind:
				fallthrough
			case types.Int64Kind:
				fallthrough
			case types.UntypedIntKind:
				return true
			}
		default:
			return false
		}
	}
	panic("unreachable")
}

func (c *compiler) VisitIndexExpr(expr *ast.IndexExpr) Value {
	value := c.VisitExpr(expr.X)
	// TODO handle maps, strings, slices.

	index := c.VisitExpr(expr.Index)
	if llvm_value, ok := index.(*LLVMValue); ok {
		if llvm_value.indirect {
			index = llvm_value.Deref()
		}
	}

	if !isIntType(index.Type()) {
		panic("Array index expression must evaluate to an integer")
	}

	// Is it an array? Then let's get the address of the array so we can
	// get an element.
	// TODO
	//if value.Type().TypeKind() == llvm.ArrayTypeKind {
	//    value = value.Metadata(llvm.MDKindID("address"))
	//}

	gep_indices := []llvm.Value{}

	var ptr llvm.Value
	var result_type types.Type
	typ := value.Type()
	if typ, ok := typ.(*types.Pointer); ok {
		switch typ := types.Deref(typ).(type) {
		case *types.Array:
			result_type = typ.Elt
			ptr = value.LLVMValue()
			gep_indices = append(gep_indices, llvm.ConstNull(llvm.Int32Type()))
		case *types.Slice:
			result_type = typ.Elt
			ptr = c.builder.CreateStructGEP(value.LLVMValue(), 0, "")
			ptr = c.builder.CreateLoad(ptr, "")
		default:
			panic("unimplemented")
		}
	}

	gep_indices = append(gep_indices, index.LLVMValue())
	element := c.builder.CreateGEP(ptr, gep_indices, "")
	result := c.builder.CreateLoad(element, "")
	return c.NewLLVMValue(result, result_type)
}

func (c *compiler) VisitSelectorExpr(expr *ast.SelectorExpr) Value {
	lhs := c.VisitExpr(expr.X)
	if lhs == nil {
		// The only time we should get a nil result is if the object is
		// a package.
		pkgident := (expr.X).(*ast.Ident)
		pkgscope := (pkgident.Obj.Data).(*ast.Scope)
		obj := pkgscope.Lookup(expr.Sel.String())
		return c.Resolve(obj)
	}

	// TODO handle interfaces.

	// TODO when we support embedded types, we'll need to do a breadth-first
	// search for the name, since the specification says to take the shallowest
	// field with the specified name.

	// Map name to an index.
	zero_value := llvm.ConstInt(llvm.Int32Type(), 0, false)
	indexes := make([]llvm.Value, 0)

	// If it's an indirect value, for example, a stack-allocated copy of a
	// parameter, take the base type and add a GEP index, but don't dereference
	// the value.
	indirect := false
	typ := lhs.Type()
	if lhs_, isllvm := lhs.(*LLVMValue); isllvm && lhs_.indirect {
		typ = types.Deref(typ)
		indexes = append(indexes, zero_value)
		indirect = true
	}

	var ptr_type types.Type
	if _, isptr := typ.(*types.Pointer); isptr {
		ptr_type = typ
		typ = types.Deref(typ)
		if indirect {
			lhs = lhs.(*LLVMValue).Deref()
			indirect = false
		} else {
			indexes = append(indexes, zero_value)
		}
	}

	// If it's a struct, look to see if it has a field with the specified name.
	name := expr.Sel.String()
	underlying := typ.(*types.Name).Underlying
	switch x := underlying.(type) {
	case *types.Struct:
		styp := x
		i := sort.Search(len(styp.Fields), func(i int) bool {
			return styp.Fields[i].Name >= name
		})
		if i < len(styp.Fields) && styp.Fields[i].Name == name {
			index := llvm.ConstInt(llvm.Int32Type(), uint64(i), false)
			indexes = append(indexes, index)
			llvm_value := c.builder.CreateGEP(lhs.LLVMValue(), indexes, "")
			elt_typ := styp.Fields[i].Type.(types.Type)
			value := c.NewLLVMValue(llvm_value, &types.Pointer{Base: elt_typ})
			value.indirect = true
			return value
		}
	case *types.Interface:
		iface := x
		i := sort.Search(len(iface.Methods), func(i int) bool {
			return iface.Methods[i].Name >= name
		})
		if i < len(iface.Methods) && iface.Methods[i].Name == name {
			struct_value := lhs.LLVMValue()
			receiver_value := c.builder.CreateStructGEP(struct_value, 0, "")
			fn_value := c.builder.CreateStructGEP(struct_value, i+1, "")
			receiver_type := &types.Pointer{Base: types.Int8}
			method_type := c.ObjGetType(iface.Methods[i]).(*types.Func)
			method_type.Recv = ast.NewObj(ast.Var, "")
			method_type.Recv.Type = receiver_type
			method := c.NewLLVMValue(
				c.builder.CreateBitCast(
					c.builder.CreateLoad(fn_value, ""),
					method_type.LLVMType(), ""), method_type)
			method.receiver = c.NewLLVMValue(
				c.builder.CreateLoad(receiver_value, ""), receiver_type)
			return method
		}
	}

	// Look up a method with receiver T.
	namedtype := typ.(*types.Name)
	i := sort.Search(len(namedtype.Methods), func(i int) bool {
		return namedtype.Methods[i].Name >= name
	})
	if i < len(namedtype.Methods) && namedtype.Methods[i].Name == name {
		method_obj := namedtype.Methods[i]
		receiver := lhs.(*LLVMValue)
		if indirect {
			receiver = receiver.Deref()
		}

		// Check if it's a pointer-receiver method.
		method_type := method_obj.Type.(*types.Func)
		recv_type := method_type.Recv.Type.(types.Type)
		is_ptr_method := !types.Identical(recv_type, typ)

		method := c.Resolve(method_obj).(*LLVMValue)
		if is_ptr_method {
			// From the language spec:
			//     If x is addressable and &x's method set contains m,
			//     x.m() is shorthand for (&x).m()
			if ptr_type == nil && receiver.address != nil {
				receiver = receiver.address
			}
			method.receiver = receiver
		} else {
			if ptr_type != nil {
				method.receiver = receiver.Deref()
			} else {
				method.receiver = receiver
			}
		}
		return method
	}

	panic("Shouldn't reach here (looking for " + name + ")")
}

func (c *compiler) VisitStarExpr(expr *ast.StarExpr) Value {
	// Are we dereferencing a pointer that's on the stack? Then load the stack
	// value.
	operand := c.VisitExpr(expr.X).(*LLVMValue)
	if operand.indirect {
		operand = operand.Deref()
	}

	// We don't want to immediately load the value, as we might be doing an
	// assignment rather than an evaluation. Instead, we return the pointer and
	// tell the caller to load it on demand.
	operand.indirect = true
	return operand
}

func (c *compiler) VisitExpr(expr ast.Expr) Value {
	switch x := expr.(type) {
	case *ast.BasicLit:
		return c.VisitBasicLit(x)
	case *ast.BinaryExpr:
		return c.VisitBinaryExpr(x)
	case *ast.FuncLit:
		return c.VisitFuncLit(x)
	case *ast.CompositeLit:
		return c.VisitCompositeLit(x)
	case *ast.UnaryExpr:
		return c.VisitUnaryExpr(x)
	case *ast.CallExpr:
		return c.VisitCallExpr(x)
	case *ast.IndexExpr:
		return c.VisitIndexExpr(x)
	case *ast.SelectorExpr:
		return c.VisitSelectorExpr(x)
	case *ast.StarExpr:
		return c.VisitStarExpr(x)
	case *ast.ParenExpr:
		return c.VisitExpr(x.X)
	case *ast.Ident:
		if x.Obj == nil {
			x.Obj = c.LookupObj(x.Name)
		}
		return c.Resolve(x.Obj)
	}
	panic(fmt.Sprintf("Unhandled Expr node: %s", reflect.TypeOf(expr)))
}

// vim: set ft=go :
