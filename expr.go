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
	"go/token"
	"go/types"
	"reflect"
	"sort"
)

func (c *compiler) isNilIdent(x ast.Expr) bool {
	ident, ok := x.(*ast.Ident)
	return ok && c.objects[ident] == types.Universe.Lookup("nil")
}

// Binary logical operators are handled specially, outside of the Value
// type, because of the need to perform lazy evaluation.
//
// Binary logical operators are implemented using a Phi node, which takes
// on the appropriate value depending on which basic blocks branch to it.
func (c *compiler) compileLogicalOp(op token.Token, lhs Value, rhsFunc func() Value) Value {
	lhsBlock := c.builder.GetInsertBlock()
	resultBlock := llvm.AddBasicBlock(lhsBlock.Parent(), "")
	resultBlock.MoveAfter(lhsBlock)
	rhsBlock := llvm.InsertBasicBlock(resultBlock, "")
	falseBlock := llvm.InsertBasicBlock(resultBlock, "")

	if op == token.LOR {
		c.builder.CreateCondBr(lhs.LLVMValue(), resultBlock, rhsBlock)
	} else {
		c.builder.CreateCondBr(lhs.LLVMValue(), rhsBlock, falseBlock)
	}
	c.builder.SetInsertPointAtEnd(rhsBlock)
	rhs := rhsFunc()
	rhsBlock = c.builder.GetInsertBlock() // rhsFunc may create blocks
	c.builder.CreateCondBr(rhs.LLVMValue(), resultBlock, falseBlock)
	c.builder.SetInsertPointAtEnd(falseBlock)
	c.builder.CreateBr(resultBlock)
	c.builder.SetInsertPointAtEnd(resultBlock)

	result := c.builder.CreatePHI(llvm.Int1Type(), "")
	trueValue := llvm.ConstAllOnes(llvm.Int1Type())
	falseValue := llvm.ConstNull(llvm.Int1Type())
	var values []llvm.Value
	var blocks []llvm.BasicBlock
	if op == token.LOR {
		values = []llvm.Value{trueValue, trueValue, falseValue}
		blocks = []llvm.BasicBlock{lhsBlock, rhsBlock, falseBlock}
	} else {
		values = []llvm.Value{trueValue, falseValue}
		blocks = []llvm.BasicBlock{rhsBlock, falseBlock}
	}
	result.AddIncoming(values, blocks)
	return c.NewValue(result, types.Typ[types.Bool])
}

func (c *compiler) VisitBinaryExpr(x *ast.BinaryExpr) Value {
	if x.Op == token.SHL || x.Op == token.SHR {
		c.convertUntyped(x.X, x)
	}
	if !c.convertUntyped(x.X, x.Y) {
		c.convertUntyped(x.Y, x.X)
	}
	lhs := c.VisitExpr(x.X)
	switch x.Op {
	case token.LOR, token.LAND:
		return c.compileLogicalOp(x.Op, lhs, func() Value { return c.VisitExpr(x.Y) })
	}
	return lhs.BinaryOp(x.Op, c.VisitExpr(x.Y))
}

func (c *compiler) VisitUnaryExpr(expr *ast.UnaryExpr) Value {
	value := c.VisitExpr(expr.X)
	return value.UnaryOp(expr.Op)
}

func (c *compiler) VisitCallExpr(expr *ast.CallExpr) Value {
	// Is it a type conversion?
	if len(expr.Args) == 1 && c.isType(expr.Fun) {
		typ := c.types.expr[expr].Type
		value := c.VisitExpr(expr.Args[0])
		return value.Convert(typ)
	}

	// Builtin functions.
	// Builtin function's have a special Type (types.builtin).
	//
	// Note: we do not handle unsafe.{Align,Offset,Size}of here,
	// as they are evaluated during type-checking.
	switch c.types.expr[expr.Fun].Type.(type) {
	case *types.NamedType, *types.Signature:
	default:
		ident := expr.Fun.(*ast.Ident)
		switch c.objects[ident].GetName() {
		case "copy":
			return c.VisitCopy(expr)
		case "print":
			return c.visitPrint(expr)
		case "println":
			return c.visitPrintln(expr)
		case "cap":
			return c.VisitCap(expr)
		case "len":
			return c.VisitLen(expr)
		case "new":
			return c.VisitNew(expr)
		case "make":
			return c.VisitMake(expr)
		case "append":
			return c.VisitAppend(expr)
		case "delete":
			m := c.VisitExpr(expr.Args[0]).(*LLVMValue)
			key := c.VisitExpr(expr.Args[1])
			c.mapDelete(m, key)
			return nil
		case "panic":
			var arg Value
			if len(expr.Args) > 0 {
				arg = c.VisitExpr(expr.Args[0])
			}
			c.visitPanic(arg)
			return nil
		case "recover":
			return c.visitRecover()
		case "real":
			cmplx := c.VisitExpr(expr.Args[0]).(*LLVMValue)
			return cmplx.extractComplexComponent(0)
		case "imag":
			cmplx := c.VisitExpr(expr.Args[0]).(*LLVMValue)
			return cmplx.extractComplexComponent(1)
		case "complex":
			r := c.VisitExpr(expr.Args[0]).LLVMValue()
			i := c.VisitExpr(expr.Args[1]).LLVMValue()
			typ := c.types.expr[expr].Type
			cmplx := llvm.Undef(c.types.ToLLVM(typ))
			cmplx = c.builder.CreateInsertValue(cmplx, r, 0, "")
			cmplx = c.builder.CreateInsertValue(cmplx, i, 1, "")
			return c.NewValue(cmplx, typ)
		}
	}

	// Not a type conversion, so must be a function call.
	lhs := c.VisitExpr(expr.Fun)
	fn := lhs.(*LLVMValue)
	fn_type := underlyingType(fn.Type()).(*types.Signature)

	var args []llvm.Value
	if nparams := len(fn_type.Params); nparams > 0 {
		if fn_type.IsVariadic {
			nparams--
		}
		for i := 0; i < nparams; i++ {
			value := c.VisitExpr(expr.Args[i])
			param_type := fn_type.Params[i].Type.(types.Type)
			args = append(args, value.Convert(param_type).LLVMValue())
		}
		if fn_type.IsVariadic {
			if expr.Ellipsis.IsValid() {
				// Calling f(x...). Just pass the slice directly.
				slice_value := c.VisitExpr(expr.Args[nparams]).LLVMValue()
				args = append(args, slice_value)
			} else {
				param_type := fn_type.Params[nparams].Type
				param_type = param_type.(*types.Slice).Elt
				varargs := make([]llvm.Value, 0)
				for i := nparams; i < len(expr.Args); i++ {
					value := c.VisitExpr(expr.Args[i])
					value = value.Convert(param_type)
					varargs = append(varargs, value.LLVMValue())
				}
				slice_value := c.makeLiteralSlice(varargs, param_type)
				args = append(args, slice_value)
			}
		}
	}

	var result_type types.Type
	switch len(fn_type.Results) {
	case 0: // no-op
	case 1:
		result_type = fn_type.Results[0].Type.(types.Type)
	default:
		result_type = &types.Result{Values: fn_type.Results}
	}

	var fnptr llvm.Value
	fnval := fn.LLVMValue()
	if fnval.Type().TypeKind() == llvm.PointerTypeKind {
		fnptr = fnval
	} else {
		fnptr = c.builder.CreateExtractValue(fnval, 0, "")
		context := c.builder.CreateExtractValue(fnval, 1, "")
		fntyp := fnptr.Type().ElementType()
		paramTypes := fntyp.ParamTypes()

		// If the context is not a constant null, and we're not
		// dealing with a method (where we don't care about the value
		// of the receiver), then we must conditionally call the
		// function with the additional receiver/closure.
		if !context.IsNull() {
			var nullctxblock llvm.BasicBlock
			var nonnullctxblock llvm.BasicBlock
			var endblock llvm.BasicBlock
			var nullctxresult llvm.Value
			if !context.IsConstant() && len(paramTypes) == len(args) {
				currblock := c.builder.GetInsertBlock()
				endblock = llvm.AddBasicBlock(currblock.Parent(), "")
				endblock.MoveAfter(currblock)
				nonnullctxblock = llvm.InsertBasicBlock(endblock, "")
				nullctxblock = llvm.InsertBasicBlock(nonnullctxblock, "")
				nullctx := c.builder.CreateIsNull(context, "")
				c.builder.CreateCondBr(nullctx, nullctxblock, nonnullctxblock)

				// null context case.
				c.builder.SetInsertPointAtEnd(nullctxblock)
				nullctxresult = c.builder.CreateCall(fnptr, args, "")
				c.builder.CreateBr(endblock)
				c.builder.SetInsertPointAtEnd(nonnullctxblock)
			}

			// non-null context case.
			var result llvm.Value
			args := append([]llvm.Value{context}, args...)
			if len(paramTypes) < len(args) {
				returnType := fntyp.ReturnType()
				ctxType := context.Type()
				paramTypes := append([]llvm.Type{ctxType}, paramTypes...)
				vararg := fntyp.IsFunctionVarArg()
				fntyp := llvm.FunctionType(returnType, paramTypes, vararg)
				fnptrtyp := llvm.PointerType(fntyp, 0)
				fnptr = c.builder.CreateBitCast(fnptr, fnptrtyp, "")
			}
			result = c.builder.CreateCall(fnptr, args, "")

			// If the return type is not void, create a
			// PHI node to select which value to return.
			if !nullctxresult.IsNil() {
				c.builder.CreateBr(endblock)
				c.builder.SetInsertPointAtEnd(endblock)
				if result.Type().TypeKind() != llvm.VoidTypeKind {
					phiresult := c.builder.CreatePHI(result.Type(), "")
					values := []llvm.Value{nullctxresult, result}
					blocks := []llvm.BasicBlock{nullctxblock, nonnullctxblock}
					phiresult.AddIncoming(values, blocks)
					result = phiresult
				}
			}
			return c.NewValue(result, result_type)
		}
	}
	result := c.builder.CreateCall(fnptr, args, "")
	return c.NewValue(result, result_type)
}

func (c *compiler) VisitIndexExpr(expr *ast.IndexExpr) Value {
	value := c.VisitExpr(expr.X)
	index := c.VisitExpr(expr.Index)

	typ := underlyingType(value.Type())
	if isString(typ) {
		ptr := c.builder.CreateExtractValue(value.LLVMValue(), 0, "")
		gepindices := []llvm.Value{index.LLVMValue()}
		ptr = c.builder.CreateGEP(ptr, gepindices, "")
		byteType := types.Typ[types.Byte]
		result := c.NewValue(ptr, &types.Pointer{Base: byteType})
		return result.makePointee()
	}

	// We can index a pointer to an array.
	if _, ok := typ.(*types.Pointer); ok {
		value = value.(*LLVMValue).makePointee()
		typ = underlyingType(value.Type())
	}

	switch typ := typ.(type) {
	case *types.Array:
		index := index.Convert(types.Typ[types.Int]).LLVMValue()
		var ptr llvm.Value
		value := value.(*LLVMValue)
		if value.pointer != nil {
			ptr = value.pointer.LLVMValue()
		} else {
			init := value.LLVMValue()
			ptr = c.builder.CreateAlloca(init.Type(), "")
			c.builder.CreateStore(init, ptr)
		}
		zero := llvm.ConstNull(llvm.Int32Type())
		element := c.builder.CreateGEP(ptr, []llvm.Value{zero, index}, "")
		result := c.NewValue(element, &types.Pointer{Base: typ.Elt})
		return result.makePointee()

	case *types.Slice:
		index := index.Convert(types.Typ[types.Int]).LLVMValue()
		ptr := c.builder.CreateExtractValue(value.LLVMValue(), 0, "")
		element := c.builder.CreateGEP(ptr, []llvm.Value{index}, "")
		result := c.NewValue(element, &types.Pointer{Base: typ.Elt})
		return result.makePointee()

	case *types.Map:
		value, _ = c.mapLookup(value.(*LLVMValue), index, false)
		return value
	}
	panic(fmt.Sprintf("unreachable (%s)", typ))
}

type selectorCandidate struct {
	Indices []int
	Type    types.Type
}

func (c *compiler) VisitSelectorExpr(expr *ast.SelectorExpr) Value {
	// Imported package funcs/vars.
	if ident, ok := expr.X.(*ast.Ident); ok {
		if _, ok := c.objects[ident].(*types.Package); ok {
			return c.Resolve(expr.Sel)
		}
	}

	// Method expression. Returns an unbound function pointer.
	// FIXME this is just the most basic case. It's also possible to
	// create a pointer-receiver function from a method that has a
	// value receiver (see Method Expressions in spec).
	if c.isType(expr.X) {
		ftyp := c.types.expr[expr].Type.(*types.Signature)
		recvtyp := ftyp.Params[0].Type
		var name *types.NamedType
		if ptrtyp, ok := recvtyp.(*types.Pointer); ok {
			name = ptrtyp.Base.(*types.NamedType)
		} else {
			name = recvtyp.(*types.NamedType)
		}
		obj := c.methods(name).Lookup(expr.Sel.Name)
		method := c.Resolve(c.objectdata[obj].Ident).(*LLVMValue)
		return c.NewValue(method.value, ftyp)
	}

	// Interface: search for method by name.
	lhs := c.VisitExpr(expr.X)
	name := expr.Sel.Name
	if iface, ok := underlyingType(lhs.Type()).(*types.Interface); ok {
		i := sort.Search(len(iface.Methods), func(i int) bool {
			return iface.Methods[i].Name >= name
		})
		structValue := lhs.LLVMValue()
		receiver := c.builder.CreateExtractValue(structValue, 1, "")
		f := c.builder.CreateExtractValue(structValue, i+2, "")
		ftype := iface.Methods[i].Type
		types := []llvm.Type{f.Type(), receiver.Type()}
		llvmStructType := llvm.StructType(types, false)
		structValue = llvm.Undef(llvmStructType)
		structValue = c.builder.CreateInsertValue(structValue, f, 0, "")
		structValue = c.builder.CreateInsertValue(structValue, receiver, 1, "")
		return c.NewValue(structValue, ftype)
	}

	// Otherwise, search for field/method,
	// recursing through embedded types.
	var recv *types.NamedType
	var result selectorCandidate
	curr := []selectorCandidate{{nil, lhs.Type()}}
	for result.Type == nil && len(curr) > 0 {
		var next []selectorCandidate
		for _, candidate := range curr {
			indices := candidate.Indices[0:]
			t := derefType(candidate.Type)

			if n, ok := t.(*types.NamedType); ok {
				for _, m := range n.Methods {
					if m.Name == name {
						recv = n
						result.Indices = indices
						result.Type = t
						break
					}
				}
			}

			if t, ok := underlyingType(t).(*types.Struct); ok {
				if i := fieldIndex(t, name); i != -1 {
					result.Indices = append(indices, i)
					result.Type = t.Fields[i].Type
					break
				} else {
					// Add embedded types to the next set of types
					// to check.
					for i, field := range t.Fields {
						if field.IsAnonymous {
							indices = append(indices[0:], i)
							t := field.Type
							candidate := selectorCandidate{indices, t}
							next = append(next, candidate)
						}
					}
				}
			}
		}
		curr = next
	}

	// Get a pointer to the field/receiver.
	recvValue := lhs.(*LLVMValue)
	if len(result.Indices) > 0 {
		if _, ok := underlyingType(lhs.Type()).(*types.Pointer); !ok {
			if recvValue.pointer != nil {
				recvValue = recvValue.pointer
			} else {
				// XXX Temporary hack: if we've got a temporary
				// (i.e. no pointer), then load the value onto
				// the stack. Later, we can just extract the
				// values.
				v := recvValue.value
				stackptr := c.builder.CreateAlloca(v.Type(), "")
				c.builder.CreateStore(v, stackptr)
				ptrtyp := &types.Pointer{Base: recvValue.Type()}
				recvValue = c.NewValue(stackptr, ptrtyp)
			}
		}
		for _, v := range result.Indices {
			ptr := recvValue.LLVMValue()
			field := underlyingType(derefType(recvValue.typ)).(*types.Struct).Fields[v]
			fieldPtr := c.builder.CreateStructGEP(ptr, v, "")
			fieldPtrTyp := &types.Pointer{Base: field.Type.(types.Type)}
			recvValue = c.NewValue(fieldPtr, fieldPtrTyp)

			// GEP returns a pointer; if the field is a pointer,
			// we must load our pointer-to-a-pointer.
			if _, ok := field.Type.(*types.Pointer); ok {
				recvValue = recvValue.makePointee()
			}
		}
	}

	// Method?
	if recv != nil {
		obj := c.methods(recv).Lookup(expr.Sel.Name)
		method := c.Resolve(c.objectdata[obj].Ident).(*LLVMValue)
		methodType := method.Type().(*types.Signature)
		receiverType := methodType.Recv.Type.(types.Type)
		var receiver Value
		switch {
		case isIdentical(recvValue.Type(), receiverType):
			receiver = recvValue
		case isIdentical(&types.Pointer{Base: recvValue.Type()}, receiverType):
			receiver = recvValue.pointer
		default:
			receiver = recvValue.makePointee()
		}
		methodValue := method.LLVMValue()
		methodValue = c.builder.CreateExtractValue(methodValue, 0, "")
		receiverValue := receiver.LLVMValue()
		types := []llvm.Type{methodValue.Type(), receiverValue.Type()}
		structType := llvm.StructType(types, false)
		value := llvm.Undef(structType)
		value = c.builder.CreateInsertValue(value, methodValue, 0, "")
		value = c.builder.CreateInsertValue(value, receiverValue, 1, "")
		return c.NewValue(value, methodType)
	} else {
		if isIdentical(recvValue.Type(), result.Type) {
			// no-op
		} else if isIdentical(&types.Pointer{Base: recvValue.Type()}, result.Type) {
			recvValue = recvValue.pointer
		} else {
			recvValue = recvValue.makePointee()
		}
		return recvValue
	}
	panic("unreachable")
}

func (c *compiler) VisitStarExpr(expr *ast.StarExpr) Value {
	switch operand := c.VisitExpr(expr.X).(type) {
	case *LLVMValue:
		// We don't want to immediately load the value, as we might be doing an
		// assignment rather than an evaluation. Instead, we return the pointer
		// and tell the caller to load it on demand.
		return operand.makePointee()
	}
	panic("unreachable")
}

func (c *compiler) VisitTypeAssertExpr(expr *ast.TypeAssertExpr) Value {
	typ := c.types.expr[expr].Type
	lhs := c.VisitExpr(expr.X)
	return lhs.Convert(typ)
}

func (c *compiler) VisitExpr(expr ast.Expr) Value {
	// Before all else, check if we've got a constant expression.
	// go/types performs constant folding, and we store the value
	// alongside the expression's type.
	if info := c.types.expr[expr]; info.Value != nil {
		return c.NewConstValue(info.Value, info.Type)
	}

	switch x := expr.(type) {
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
	case *ast.TypeAssertExpr:
		return c.VisitTypeAssertExpr(x)
	case *ast.SliceExpr:
		return c.VisitSliceExpr(x)
	case *ast.Ident:
		return c.Resolve(x)
	}
	panic(fmt.Sprintf("Unhandled Expr node: %s", reflect.TypeOf(expr)))
}

// vim: set ft=go :
