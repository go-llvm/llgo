// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo/types"
)

func (c *compiler) visitRecover() *LLVMValue {
	// TODO
	errval := llvm.ConstNull(c.types.ToLLVM(types.Error))
	return c.NewLLVMValue(errval, types.Error)
}

