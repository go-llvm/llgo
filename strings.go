package llgo

import (
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo/types"
)

func (c *compiler) concatenateStrings(lhs, rhs *LLVMValue) *LLVMValue {
	strcat := c.module.Module.NamedFunction("runtime.strcat")
	if strcat.IsNil() {
		string_type := c.types.ToLLVM(types.String)
		param_types := []llvm.Type{string_type, string_type}
		func_type := llvm.FunctionType(string_type, param_types, false)
		strcat = llvm.AddFunction(c.module.Module, "runtime.strcat", func_type)
	}
	args := []llvm.Value{lhs.LLVMValue(), rhs.LLVMValue()}
	result := c.builder.CreateCall(strcat, args, "")
	return c.NewLLVMValue(result, types.String)
}

// vim: set ft=go:
