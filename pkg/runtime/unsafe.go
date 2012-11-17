// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

// #llgo name: llvm.trap
func llvm_trap()

// #llgo name: reflect.unsafe_New
func unsafe_New(t type_) unsafe.Pointer {
	// TODO
	println("TODO: unsafe_New")
	llvm_trap()
	return 0
}

// #llgo name: reflect.unsafe_NewArray
func unsafe_NewArray(t type_, n int) unsafe.Pointer {
	// TODO
	println("TODO: unsafe_NewArray")
	llvm_trap()
	return 0
}
