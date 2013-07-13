// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"fmt"
	"github.com/axw/gollvm/llvm"
	"go/ast"
)

func getprintf(module llvm.Module) llvm.Value {
	printf := module.NamedFunction("printf")
	if printf.IsNil() {
		CharPtr := llvm.PointerType(llvm.Int8Type(), 0)
		fn_type := llvm.FunctionType(
			llvm.Int32Type(), []llvm.Type{CharPtr}, true)
		printf = llvm.AddFunction(module, "printf", fn_type)
		printf.SetFunctionCallConv(llvm.CCallConv)
	}
	return printf
}

func (c *compiler) getBoolString(v llvm.Value) llvm.Value {
	startBlock := c.builder.GetInsertBlock()
	resultBlock := llvm.InsertBasicBlock(startBlock, "")
	resultBlock.MoveAfter(startBlock)
	falseBlock := llvm.InsertBasicBlock(resultBlock, "")

	CharPtr := llvm.PointerType(llvm.Int8Type(), 0)
	falseString := c.builder.CreateGlobalStringPtr("false", "")
	falseString = c.builder.CreateBitCast(falseString, CharPtr, "")
	trueString := c.builder.CreateGlobalStringPtr("true", "")
	trueString = c.builder.CreateBitCast(trueString, CharPtr, "")

	c.builder.CreateCondBr(v, resultBlock, falseBlock)
	c.builder.SetInsertPointAtEnd(falseBlock)
	c.builder.CreateBr(resultBlock)
	c.builder.SetInsertPointAtEnd(resultBlock)
	result := c.builder.CreatePHI(CharPtr, "")
	result.AddIncoming([]llvm.Value{trueString, falseString},
		[]llvm.BasicBlock{startBlock, falseBlock})
	return result
}

func (c *compiler) printValues(println_ bool, values ...Value) Value {
	var args []llvm.Value = nil
	if len(values) > 0 {
		format := ""
		args = make([]llvm.Value, 0, len(values)+1)
		for i, value := range values {
			llvm_value := value.LLVMValue()

			typ := value.Type().Underlying()
			if name, isname := typ.(*types.Named); isname {
				typ = name.Underlying()
			}

			if println_ && i > 0 {
				format += " "
			}
			switch typ := typ.(type) {
			case *types.Basic:
				switch typ.Kind() {
				case types.Uint8:
					format += "%hhu"
				case types.Uint16:
					format += "%hu"
				case types.Uint32:
					format += "%u"
				case types.Uintptr, types.Uint:
					format += "%lu"
				case types.Uint64:
					format += "%llu" // FIXME windows
				case types.Int:
					format += "%ld"
				case types.Int8:
					format += "%hhd"
				case types.Int16:
					format += "%hd"
				case types.Int32:
					format += "%d"
				case types.Int64:
					format += "%lld" // FIXME windows
				case types.Float32:
					llvm_value = c.builder.CreateFPExt(llvm_value, llvm.DoubleType(), "")
					fallthrough
				case types.Float64:
					printfloat := c.NamedFunction("runtime.printfloat", "func(float64) string")
					args := []llvm.Value{llvm_value}
					llvm_value = c.builder.CreateCall(printfloat, args, "")
					fallthrough
				case types.String, types.UntypedString:
					ptrval := c.builder.CreateExtractValue(llvm_value, 0, "")
					lenval := c.builder.CreateExtractValue(llvm_value, 1, "")
					llvm_value = ptrval
					args = append(args, lenval)
					format += "%.*s"
				case types.Bool:
					format += "%s"
					llvm_value = c.getBoolString(llvm_value)
				case types.UnsafePointer:
					format += "%p"
				default:
					panic(fmt.Sprint("Unhandled Basic Kind: ", typ.Kind))
				}

			case *types.Interface:
				format += "(0x%lx,0x%lx)"
				ival := c.builder.CreateExtractValue(llvm_value, 0, "")
				itype := c.builder.CreateExtractValue(llvm_value, 1, "")
				args = append(args, ival)
				llvm_value = itype

			case *types.Slice, *types.Array:
				// If we see a constant array, we either:
				//     Create an internal constant if it's a constant array, or
				//     Create space on the stack and store it there.
				init_ := value.(*LLVMValue)
				if init_.pointer != nil {
					llvm_value = init_.pointer.LLVMValue()
				} else {
					init_value := init_.LLVMValue()
					llvm_value = c.builder.CreateAlloca(init_value.Type(), "")
					c.builder.CreateStore(init_value, llvm_value)
				}
				// FIXME don't assume string...
				format += "%s"

			case *types.Pointer:
				format += "0x%lx"

			default:
				panic(fmt.Sprintf("Unhandled type kind: %s (%T)", typ, typ))
			}

			args = append(args, llvm_value)
		}
		if println_ {
			format += "\n"
		}
		formatval := c.builder.CreateGlobalStringPtr(format, "")
		args = append([]llvm.Value{formatval}, args...)
	} else {
		var format string
		if println_ {
			format = "\n"
		}
		args = []llvm.Value{c.builder.CreateGlobalStringPtr(format, "")}
	}
	printf := getprintf(c.module.Module)
	result := c.NewValue(c.builder.CreateCall(printf, args, ""), types.Typ[types.Int32])
	fflush := c.NamedFunction("fflush", "func(*int32) int32")
	c.builder.CreateCall(fflush, []llvm.Value{llvm.ConstNull(llvm.PointerType(llvm.Int32Type(), 0))}, "")
	return result
}

func (c *compiler) visitPrint(expr *ast.CallExpr) Value {
	var values []Value
	for _, arg := range expr.Args {
		values = append(values, c.VisitExpr(arg))
	}
	return c.printValues(false, values...)
}

func (c *compiler) visitPrintln(expr *ast.CallExpr) Value {
	var values []Value
	for _, arg := range expr.Args {
		values = append(values, c.VisitExpr(arg))
	}
	return c.printValues(true, values...)
}

// vim: set ft=go :
