// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"github.com/axw/gollvm/llvm"
)

// coerce yields a value of the the type specified, initialised
// to the exact bit pattern as in the specified value.
//
// Note: the value's type and the specified target type must have
// the same size. If the source is an aggregate, then the target
// must also be an aggregate with the same number of fields, each
// of which must have the same size.
func (c *compiler) coerce(v llvm.Value, t llvm.Type) llvm.Value {
	// FIXME don't do this with alloca
	switch t.TypeKind() {
	case llvm.ArrayTypeKind, llvm.StructTypeKind:
		ptr := c.builder.CreateAlloca(t, "")
		ptrv := c.builder.CreateBitCast(ptr, llvm.PointerType(v.Type(), 0), "")
		c.builder.CreateStore(v, ptrv)
		return c.builder.CreateLoad(ptr, "")
	}

	vt := v.Type()
	switch vt.TypeKind() {
	case llvm.ArrayTypeKind, llvm.StructTypeKind:
		ptr := c.builder.CreateAlloca(vt, "")
		c.builder.CreateStore(v, ptr)
		ptrt := c.builder.CreateBitCast(ptr, llvm.PointerType(t, 0), "")
		return c.builder.CreateLoad(ptrt, "")
	}

	return c.builder.CreateBitCast(v, t, "")
}
