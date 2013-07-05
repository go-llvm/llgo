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

func streqalg(size uintptr, lhs, rhs unsafe.Pointer) bool {
	a := (*_string)(lhs)
	b := (*_string)(rhs)
	return strcmp(*a, *b) == 0
}

func f32eqalg(size uintptr, lhs, rhs unsafe.Pointer) bool {
	return *(*float32)(lhs) == *(*float32)(rhs)
}

func f64eqalg(size uintptr, lhs, rhs unsafe.Pointer) bool {
	return *(*float64)(lhs) == *(*float64)(rhs)
}

func c64eqalg(size uintptr, lhs, rhs unsafe.Pointer) bool {
	return *(*complex64)(lhs) == *(*complex64)(rhs)
}

func c128eqalg(size uintptr, lhs, rhs unsafe.Pointer) bool {
	return *(*complex128)(lhs) == *(*complex128)(rhs)
}
