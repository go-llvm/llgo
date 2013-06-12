// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"github.com/axw/gollvm/llvm"
)

type Builder struct {
	llvm.Builder
	types *TypeMap
}

func newBuilder(tm *TypeMap) *Builder {
	return &Builder{
		Builder: llvm.GlobalContext().NewBuilder(),
		types:   tm,
	}
}

func (b *Builder) CreateLoad(v llvm.Value, name string) llvm.Value {
	result := b.Builder.CreateLoad(v, name)
	if !b.types.ptrstandin.IsNil() && result.Type() == b.types.ptrstandin {
		// We represent recursive pointer types (T = *T)
		// in LLVM as a pointer to "ptrstdin", where
		// ptrstandin is a pointer to a unique named struct.
		//
		// Cast the result of loading the pointer back to
		// the same type as the pointer loaded from.
		result = b.CreateBitCast(result, v.Type(), "")
	}
	return result
}

func (b *Builder) CreateStore(v, ptr llvm.Value) {
	if !b.types.ptrstandin.IsNil() {
		vtyp, ptrtyp := v.Type(), ptr.Type()
		if vtyp == ptrtyp || ptrtyp.ElementType() == b.types.ptrstandin {
			// We must be dealing with a pointer to a recursive pointer
			// type, so bitcast the value to the pointer's base, opaque
			// pointer type.
			v = b.CreateBitCast(v, ptrtyp.ElementType(), "")
		}
	}
	b.Builder.CreateStore(v, ptr)
}
