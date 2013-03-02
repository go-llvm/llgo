// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

func eqalg(fn unsafe.Pointer, size uintptr, a unsafe.Pointer, b unsafe.Pointer) bool

func memequal(size uintptr, lhs, rhs unsafe.Pointer) bool {
	if lhs == rhs {
		return true
	}
	a := uintptr(lhs)
	b := uintptr(rhs)
	end := a + size
	for a != end {
		lhs, rhs := (*byte)(unsafe.Pointer(a)), (*byte)(unsafe.Pointer(b))
		if *lhs != *rhs {
			return false
		}
		a++
		b++
	}
	return true
}
