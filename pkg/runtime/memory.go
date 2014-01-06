// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

// #llgo name: malloc
func c_malloc(uintptr) *int8

func malloc(size uintptr) unsafe.Pointer {
	mem := unsafe.Pointer(c_malloc(size))
	if mem != nil {
		bzero(mem, size)
	}
	return mem
}

func free(unsafe.Pointer)
func memcpy(dst, src unsafe.Pointer, size uintptr)
func memmove(dst, src unsafe.Pointer, size uintptr)
func memset(dst unsafe.Pointer, fill byte, size uintptr)

// #llgo name: llvm.stackrestore
func llvm_stackrestore(*int8)

// #llgo name: llvm.stacksave
func llvm_stacksave() *int8

func bzero(dst unsafe.Pointer, size uintptr) {
	memset(dst, 0, size)
}

func align(p uintptr, align uintptr) uintptr {
	if p%align != 0 {
		p += (align - (p % align))
	}
	return p
}
