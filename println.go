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

func (c *compiler) VisitPrintln(expr *ast.CallExpr) Value {
	var args []llvm.Value = nil
	if expr.Args != nil {
		format := ""
		args = make([]llvm.Value, 0, len(expr.Args)+1)
		for i, expr := range expr.Args {
			value := c.VisitExpr(expr)
			// Is it a global variable or non-constant? Then we'll need to load
			// it if it's not a pointer to an array.
			if llvm_value, isllvm := value.(*LLVMValue); isllvm {
				if llvm_value.indirect {
					value = llvm_value.Deref()
				}
			}
			llvm_value := value.LLVMValue()

			// If it's a named type, get the underlying type.
			typ := value.Type()
			if name, isname := typ.(*types.Name); isname {
				typ = name.Underlying
			}

			if i > 0 {
				format += " "
			}
			switch typ := typ.(type) {
			case *types.Basic:
				switch typ.Kind {
				case types.Uint16Kind:
					format += "%hu"
				case types.Uint32Kind:
					format += "%u"
				case types.Uint64Kind:
					format += "%llu" // FIXME windows
				case types.Int16Kind:
					format += "%hd"
				case types.Int32Kind:
					format += "%d"
				case types.Int64Kind:
					format += "%lld" // FIXME windows
				case types.StringKind:
					ptrval := c.builder.CreateExtractValue(llvm_value, 0, "")
					lenval := c.builder.CreateExtractValue(llvm_value, 1, "")
					llvm_value = ptrval
					args = append(args, lenval)
					format += "%*s"
				case types.BoolKind:
					format += "%d"
					llvm_value = c.builder.CreateZExt(llvm_value, llvm.Int32Type(), "")
				default:
					panic(fmt.Sprint("Unhandled Basic Kind: ", typ.Kind))
				}

			case *types.Slice, *types.Array:
				// If we see a constant array, we either:
				//     Create an internal constant if it's a constant array, or
				//     Create space on the stack and store it there.
				init_ := value
				init_value := init_.LLVMValue()
				switch init_.(type) {
				case ConstValue:
					llvm_value = llvm.AddGlobal(
						c.module.Module, init_value.Type(), "")
					llvm_value.SetInitializer(init_value)
					llvm_value.SetGlobalConstant(true)
					llvm_value.SetLinkage(llvm.InternalLinkage)
				case *LLVMValue:
					llvm_value = c.builder.CreateAlloca(
						init_value.Type(), "")
					c.builder.CreateStore(init_value, llvm_value)
				}
				// FIXME don't assume string...
				format += "%s"

			case *types.Pointer:
				format += "0x%x"

			default:
				panic(fmt.Sprint("Unhandled type kind: ", typ))
			}

			args = append(args, llvm_value)
		}
		format += "\n"
		formatval := c.builder.CreateGlobalStringPtr(format, "")
		args = append([]llvm.Value{formatval}, args...)
	} else {
		args = []llvm.Value{c.builder.CreateGlobalStringPtr("\n", "")}
	}

	printf := getprintf(c.module.Module)
	return c.NewLLVMValue(c.builder.CreateCall(printf, args, ""), types.Int32)
}

// vim: set ft=go :
