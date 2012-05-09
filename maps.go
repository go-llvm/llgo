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
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo/types"
)

// mapInsert inserts a key into a map, returning a pointer to the memory
// location for the value.
func (c *compiler) mapInsert(m *LLVMValue, key Value) *LLVMValue {
	mapType := m.Type().(*types.Map)
	mapinsert := c.module.Module.NamedFunction("runtime.mapinsert")
	ptrType := c.target.IntPtrType()
	if mapinsert.IsNil() {
		// params: dynamic type, mapptr, keyptr
		paramTypes := []llvm.Type{ptrType, ptrType, ptrType}
		funcType := llvm.FunctionType(ptrType, paramTypes, false)
		mapinsert = llvm.AddFunction(c.module.Module, "runtime.mapinsert", funcType)
	}
	args := make([]llvm.Value, 3)
	args[0] = llvm.ConstPtrToInt(c.types.ToRuntime(m.Type()), ptrType)
	args[1] = c.builder.CreatePtrToInt(m.address.LLVMValue(), ptrType, "")

	if lv, islv := key.(*LLVMValue); islv {
		if lv.indirect {
			args[2] = c.builder.CreatePtrToInt(key.LLVMValue(), ptrType, "")
		} else if lv.address != nil {
			args[2] = c.builder.CreatePtrToInt(lv.address.LLVMValue(), ptrType, "")
		}
	}
	if args[2].IsNil() {
		// Create global constant, so we can take its address.
		global := llvm.AddGlobal(c.module.Module, c.types.ToLLVM(key.Type()), "")
		global.SetGlobalConstant(true)
		global.SetInitializer(key.LLVMValue())
		args[2] = c.builder.CreatePtrToInt(global, ptrType, "")
	}

	eltPtrType := &types.Pointer{Base: mapType.Elt}
	result := c.builder.CreateCall(mapinsert, args, "")
	result = c.builder.CreateIntToPtr(result, c.types.ToLLVM(eltPtrType), "")
	value := c.NewLLVMValue(result, eltPtrType)
	value.indirect = true
	return value
}

// vim: set ft=go:
