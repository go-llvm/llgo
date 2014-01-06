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
func coerce(b llvm.Builder, v llvm.Value, t llvm.Type) llvm.Value {
	// FIXME don't do this with alloca
	switch t.TypeKind() {
	case llvm.ArrayTypeKind, llvm.StructTypeKind:
		ptr := b.CreateAlloca(t, "")
		ptrv := b.CreateBitCast(ptr, llvm.PointerType(v.Type(), 0), "")
		b.CreateStore(v, ptrv)
		return b.CreateLoad(ptr, "")
	}
	vt := v.Type()
	switch vt.TypeKind() {
	case llvm.ArrayTypeKind, llvm.StructTypeKind:
		ptr := b.CreateAlloca(vt, "")
		b.CreateStore(v, ptr)
		ptrt := b.CreateBitCast(ptr, llvm.PointerType(t, 0), "")
		return b.CreateLoad(ptrt, "")
	}
	return b.CreateBitCast(v, t, "")
}
