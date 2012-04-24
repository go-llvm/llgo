package llgo

import (
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo/types"
)

func (c *compiler) concatenateStrings(lhs, rhs *LLVMValue) *LLVMValue {
	_llgo_strcat := c.module.Module.NamedFunction("_llgo_strcat")
	if _llgo_strcat.IsNil() {
		string_type := c.types.ToLLVM(types.String)
		param_types := []llvm.Type{string_type, string_type}
		func_type := llvm.FunctionType(string_type, param_types, false)
		_llgo_strcat = llvm.AddFunction(c.module.Module, "_llgo_strcat", func_type)
	}
	args := []llvm.Value{lhs.LLVMValue(), rhs.LLVMValue()}
	result := c.builder.CreateCall(_llgo_strcat, args, "")
	return c.NewLLVMValue(result, types.String)
}

// vim: set ft=go:
