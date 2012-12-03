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
	"./types"
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

			typ := types.Underlying(value.Type())
			if name, isname := typ.(*types.Name); isname {
				typ = name.Underlying
			}

			if println_ && i > 0 {
				format += " "
			}
			switch typ := typ.(type) {
			case *types.Basic:
				switch typ.Kind {
				case types.UintKind:
					format += "%u" // TODO make 32/64-bit
				case types.Uint8Kind:
					format += "%hhu"
				case types.Uint16Kind:
					format += "%hu"
				case types.Uint32Kind, types.UintptrKind: // FIXME uintptr to become bitwidth dependent
					format += "%u"
				case types.Uint64Kind:
					format += "%llu" // FIXME windows
				case types.IntKind:
					format += "%d" // TODO make 32/64-bit
				case types.Int8Kind:
					format += "%hhd"
				case types.Int16Kind:
					format += "%hd"
				case types.Int32Kind:
					format += "%d"
				case types.Int64Kind:
					format += "%lld" // FIXME windows
				case types.Float32Kind:
					llvm_value = c.builder.CreateFPExt(llvm_value, llvm.DoubleType(), "")
					fallthrough
				case types.Float64Kind:
					// Doesn't match up with gc's formatting, which allocates
					// a minimum of three digits for the exponent.
					printfloat := c.NamedFunction("runtime.printfloat", "func f(float64) string")
					args := []llvm.Value{llvm_value}
					llvm_value = c.builder.CreateCall(printfloat, args, "")
					fallthrough
				case types.StringKind:
					ptrval := c.builder.CreateExtractValue(llvm_value, 0, "")
					lenval := c.builder.CreateExtractValue(llvm_value, 1, "")
					llvm_value = ptrval
					args = append(args, lenval)
					format += "%.*s"
				case types.BoolKind:
					format += "%s"
					llvm_value = c.getBoolString(llvm_value)
				case types.UnsafePointerKind:
					format += "%p"
				default:
					panic(fmt.Sprint("Unhandled Basic Kind: ", typ.Kind))
				}

			case *types.Interface:
				format += "(%p,%p)"
				ival := c.builder.CreateExtractValue(llvm_value, 0, "")
				itype := c.builder.CreateExtractValue(llvm_value, 1, "")
				args = append(args, ival)
				llvm_value = itype

			case *types.Slice, *types.Array:
				// If we see a constant array, we either:
				//     Create an internal constant if it's a constant array, or
				//     Create space on the stack and store it there.
				init_ := value
				init_value := init_.LLVMValue()
				switch init_.(type) {
				case ConstValue:
					llvm_value = llvm.AddGlobal(c.module.Module, init_value.Type(), "")
					llvm_value.SetInitializer(init_value)
					llvm_value.SetGlobalConstant(true)
				case *LLVMValue:
					llvm_value = c.builder.CreateAlloca(init_value.Type(), "")
					c.builder.CreateStore(init_value, llvm_value)
				}
				// FIXME don't assume string...
				format += "%s"

			case *types.Pointer:
				format += "0x%x"

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
	result := c.NewLLVMValue(c.builder.CreateCall(printf, args, ""), types.Int32)
	fflush := c.NamedFunction("fflush", "func f(*int32) int32")
	c.builder.CreateCall(fflush, []llvm.Value{llvm.ConstNull(llvm.PointerType(llvm.Int32Type(), 0))}, "")
	return result
}

func (c *compiler) VisitPrint(expr *ast.CallExpr, println_ bool) Value {
	var values []Value
	for _, arg := range expr.Args {
		values = append(values, c.VisitExpr(arg))
	}
	return c.printValues(println_, values...)
}

// vim: set ft=go :
