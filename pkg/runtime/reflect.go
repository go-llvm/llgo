// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

type runtimeIword unsafe.Pointer
type runtimeSelect struct {
	dir uintptr      // 0, SendDir, or RecvDir
	typ *rtype       // channel type
	ch  runtimeIword // interface word for channel
	val runtimeIword // interface word for value (for SendDir)
}

// #llgo name: reflect.call
func reflect_call(fn, arg unsafe.Pointer, n uint32) {
	// TODO This is going to get messy. This code will
	// obviously need to know the calling convention,
	// which we currently don't specify, so as to have
	// LLVM generate the most efficient code it can.
	//
	// Apart from the obvious option of specifying a
	// calling convention (C, I suppose), there is
	// another somewhat horrible option: generate a
	// separate shim function with a specific calling
	// convention specifically for reflection.
	panic("unimplemented")
}

// #llgo name: reflect.cacheflush
func reflect_cacheflush(start, end *byte) {
	panic("unimplemented")
}

// #llgo name: reflect.makeFuncStub
func reflect_makeFuncStub() {
	panic("unimplemented")
}

// #llgo name: reflect.rselect
func reflect_rselect([]runtimeSelect) (chosen int, recv runtimeIword, recvOK bool) {
	panic("unimplemented")
}

// #llgo name: reflect.typelinks
func reflect_typelinks() []*rtype {
	panic("unimplemented")
}
