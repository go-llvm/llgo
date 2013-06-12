// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"fmt"
	"github.com/greggoryhz/gollvm/llvm"
	"go/ast"
	"go/token"
	"reflect"
	"sort"
)

func (c *compiler) isNilIdent(x ast.Expr) bool {
	ident, ok := x.(*ast.Ident)
	return ok && c.objects[ident] == types.Universe.Lookup(nil, "nil")
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

func (c *compiler) evalCallArgs(ftype *types.Signature, args []ast.Expr) []Value {
	var argValues []Value
	if len(args) == 0 {
		return argValues
	}
	arg0 := args[0]
	if _, ok := c.types.expr[arg0].Type.(*types.Tuple); ok {
		// f(g(...)), where g is multi-value return
		argValues = c.destructureExpr(args[0])
	} else {
		argValues = make([]Value, len(args))
		for i, x := range args {
			var paramtyp types.Type
			params := ftype.Params()
			if ftype.IsVariadic() && i >= int(params.Len()-1) {
				paramtyp = params.At(int(params.Len() - 1)).Type()
			} else {
				paramtyp = params.At(i).Type()
			}
			c.convertUntyped(x, paramtyp)
			argValues[i] = c.VisitExpr(x)
		}
	}
	return argValues
}

func (c *compiler) VisitCallExpr(expr *ast.CallExpr) Value {
	// Is it a type conversion?
	if len(expr.Args) == 1 && c.isType(expr.Fun) {
		typ := c.types.expr[expr].Type
		c.convertUntyped(expr.Args[0], typ)
		value := c.VisitExpr(expr.Args[0])
		return value.Convert(typ)
	}

	// Builtin functions.
	// Builtin function's have a special Type (types.builtin).
	//
	// Note: we do not handle unsafe.{Align,Offset,Size}of here,
	// as they are evaluated during type-checking.
	switch c.types.expr[expr.Fun].Type.(type) {
	case *types.Named, *types.Signature:
	default:
		ident := expr.Fun.(*ast.Ident)
		switch c.objects[ident].Name() {
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
	fn_type := fn.Type().Underlying().(*types.Signature)

	// Evaluate arguments.
	argValues := c.evalCallArgs(fn_type, expr.Args)

	// Depending on whether the function contains defer statements or not,
	// we'll generate either a "call" or an "invoke" instruction.
	var invoke bool
	if f := c.functions.top(); f != nil && !f.deferblock.IsNil() {
		invoke = true
	}
	dotdotdot := expr.Ellipsis.IsValid()
	return c.createCall(fn, argValues, dotdotdot, invoke)
}

// createCall emits the code for a function call, taking into account
// variadic functions, receivers, and panic/defer.
//
// dotdotdot is true if the last argument is followed with "...".
func (c *compiler) createCall(fn *LLVMValue, argValues []Value, dotdotdot, invoke bool) *LLVMValue {
	fn_type := fn.Type().Underlying().(*types.Signature)
	var args []llvm.Value

	// TODO Move all of this to evalCallArgs?
	params := fn_type.Params()
	if nparams := int(params.Len()); nparams > 0 {
		if fn_type.IsVariadic() {
			nparams--
		}
		for i := 0; i < nparams; i++ {
			value := argValues[i]
			param_type := params.At(i).Type()
			args = append(args, value.Convert(param_type).LLVMValue())
		}
		if fn_type.IsVariadic() {
			if dotdotdot {
				// Calling f(x...). Just pass the slice directly.
				slice_value := argValues[nparams].LLVMValue()
				args = append(args, slice_value)
			} else {
				param_type := params.At(nparams).Type()
				varargs := make([]llvm.Value, 0)
				for _, value := range argValues[nparams:] {
					value = value.Convert(param_type)
					varargs = append(varargs, value.LLVMValue())
				}
				slice_value := c.makeLiteralSlice(varargs, param_type)
				args = append(args, slice_value)
			}
		}
	}

	var result_type types.Type
	switch results := fn_type.Results(); results.Len() {
	case 0: // no-op
	case 1:
		result_type = results.At(0).Type()
	default:
		result_type = results
	}

	// Depending on whether the function contains defer statements or not,
	// we'll generate either a "call" or an "invoke" instruction.
	var createCall = c.builder.CreateCall
	if invoke {
		f := c.functions.top()
		// TODO Create a method on compiler (avoid creating closures).
		createCall = func(fn llvm.Value, args []llvm.Value, name string) llvm.Value {
			currblock := c.builder.GetInsertBlock()
			returnblock := llvm.AddBasicBlock(currblock.Parent(), "")
			returnblock.MoveAfter(currblock)
			value := c.builder.CreateInvoke(fn, args, returnblock, f.unwindblock, "")
			c.builder.SetInsertPointAtEnd(returnblock)
			return value
		}
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
		if !context.IsNull() || fn_type.Recv() != nil {
			// Store the blocks for referencing in the Phi below;
			// note that we update the block after each createCall,
			// since createCall may create new blocks and we want
			// the predecessors to the Phi.
			var nullctxblock llvm.BasicBlock
			var nonnullctxblock llvm.BasicBlock
			var endblock llvm.BasicBlock
			var nullctxresult llvm.Value

			// len(paramTypes) == len(args) iff function is not a method.
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
				nullctxresult = createCall(fnptr, args, "")
				nullctxblock = c.builder.GetInsertBlock()
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
			result = createCall(fnptr, args, "")

			// If the return type is not void, create a
			// PHI node to select which value to return.
			if !nullctxresult.IsNil() {
				nonnullctxblock = c.builder.GetInsertBlock()
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
	result := createCall(fnptr, args, "")
	return c.NewValue(result, result_type)
}

func (c *compiler) VisitIndexExpr(expr *ast.IndexExpr) Value {
	value := c.VisitExpr(expr.X)
	index := c.VisitExpr(expr.Index)

	typ := value.Type().Underlying()
	if isString(typ) {
		ptr := c.builder.CreateExtractValue(value.LLVMValue(), 0, "")
		gepindices := []llvm.Value{index.LLVMValue()}
		ptr = c.builder.CreateGEP(ptr, gepindices, "")
		byteType := types.Typ[types.Byte]
		result := c.NewValue(ptr, types.NewPointer(byteType))
		return result.makePointee()
	}

	// We can index a pointer to an array.
	if _, ok := typ.(*types.Pointer); ok {
		value = value.(*LLVMValue).makePointee()
		typ = value.Type().Underlying()
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
		result := c.NewValue(element, types.NewPointer(typ.Elem()))
		return result.makePointee()

	case *types.Slice:
		index := index.Convert(types.Typ[types.Int]).LLVMValue()
		ptr := c.builder.CreateExtractValue(value.LLVMValue(), 0, "")
		element := c.builder.CreateGEP(ptr, []llvm.Value{index}, "")
		result := c.NewValue(element, types.NewPointer(typ.Elem()))
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
	if c.isType(expr.X) {
		ftyp := c.types.expr[expr].Type.(*types.Signature)
		recvtyp := ftyp.Params().At(0).Type()
		var name *types.Named
		var isptr bool
		if ptrtyp, ok := recvtyp.(*types.Pointer); ok {
			isptr = true
			name = ptrtyp.Elem().(*types.Named)
		} else {
			name = recvtyp.(*types.Named)
		}
		obj := c.methods(name).lookup(expr.Sel.Name, isptr)
		method := c.Resolve(c.objectdata[obj].Ident).(*LLVMValue)
		return c.NewValue(method.value, ftyp)
	}

	// Interface: search for method by name.
	lhs := c.VisitExpr(expr.X)
	name := expr.Sel.Name
	if iface, ok := lhs.Type().Underlying().(*types.Interface); ok {
		methods := sortedMethods(iface)
		i := sort.Search(len(methods), func(i int) bool {
			return methods[i].Name() >= name
		})
		structValue := lhs.LLVMValue()
		receiver := c.builder.CreateExtractValue(structValue, 1, "")
		f := c.builder.CreateExtractValue(structValue, i+2, "")
		ftype := methods[i].Type()
		types := []llvm.Type{f.Type(), receiver.Type()}
		llvmStructType := llvm.StructType(types, false)
		structValue = llvm.Undef(llvmStructType)
		structValue = c.builder.CreateInsertValue(structValue, f, 0, "")
		structValue = c.builder.CreateInsertValue(structValue, receiver, 1, "")
		return c.NewValue(structValue, ftype)
	}

	// Method.
	if typ, ok := c.types.expr[expr].Type.(*types.Signature); ok && typ.Recv() != nil {
		var isptr bool
		typ := lhs.Type()
		if ptr, ok := typ.(*types.Pointer); ok {
			typ = ptr.Elem()
			isptr = true
		} else {
			isptr = lhs.(*LLVMValue).pointer != nil
		}
		method := c.methods(typ).lookup(name, isptr)
		if method != nil {
			recv := lhs.(*LLVMValue)
			if isptr && typ == lhs.Type() {
				recv = recv.pointer
			}

			if f, ok := method.(*types.Func); ok {
				method = c.methodfunc(f)
			}
			methodValue := c.Resolve(c.objectdata[method].Ident).LLVMValue()
			methodValue = c.builder.CreateExtractValue(methodValue, 0, "")
			recvValue := recv.LLVMValue()
			types := []llvm.Type{methodValue.Type(), recvValue.Type()}
			structType := llvm.StructType(types, false)
			value := llvm.Undef(structType)
			value = c.builder.CreateInsertValue(value, methodValue, 0, "")
			value = c.builder.CreateInsertValue(value, recvValue, 1, "")
			return c.NewValue(value, method.Type())
		}
	}

	// Otherwise, search for field, recursing through embedded types.
	var result selectorCandidate
	curr := []selectorCandidate{{nil, lhs.Type()}}
	for result.Type == nil && len(curr) > 0 {
		var next []selectorCandidate
		for _, candidate := range curr {
			indices := candidate.Indices[:]
			t := candidate.Type
			if ptr, ok := t.(*types.Pointer); ok {
				indices = append(indices, -1)
				t = ptr.Elem()
			}
			if t, ok := t.Underlying().(*types.Struct); ok {
				if i := fieldIndex(t, name); i != -1 {
					result.Indices = append(indices, i)
					result.Type = t.Field(i).Type()
					break
				} else {
					// Add embedded types to the next set of types to check.
					for i := 0; i < t.NumFields(); i++ {
						field := t.Field(i)
						if field.Anonymous() {
							indices := append(indices[:], i)
							t := field.Type()
							candidate := selectorCandidate{indices, t}
							next = append(next, candidate)
						}
					}
				}
			}
		}
		curr = next
	}

	// Get a pointer to the field.
	fieldValue := lhs.(*LLVMValue)
	if len(result.Indices) > 0 {
		if fieldValue.pointer == nil {
			// If we've got a temporary (i.e. no pointer),
			// then load the value onto the stack.
			v := fieldValue.value
			stackptr := c.builder.CreateAlloca(v.Type(), "")
			c.builder.CreateStore(v, stackptr)
			ptrtyp := types.NewPointer(fieldValue.Type())
			fieldValue = c.NewValue(stackptr, ptrtyp).makePointee()
		}

		for _, i := range result.Indices {
			if i == -1 {
				fieldValue = fieldValue.makePointee()
			} else {
				ptr := fieldValue.pointer.LLVMValue()
				structTyp := fieldValue.typ.Deref().Underlying().(*types.Struct)
				field := structTyp.Field(i)
				fieldPtr := c.builder.CreateStructGEP(ptr, i, "")
				fieldPtrTyp := types.NewPointer(field.Type())
				fieldValue = c.NewValue(fieldPtr, fieldPtrTyp).makePointee()
			}
		}
	}
	return fieldValue
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
