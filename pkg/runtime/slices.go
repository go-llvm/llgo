// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

// sliceappend takes a slice type and two slices, and returns the
// result of appending the second slice to the first, growing the
// slice as necessary.
func sliceappend(t unsafe.Pointer, a, b slice) slice {
	slicetyp := (*sliceType)(t)
	if a.cap-a.len < b.len {
		a = slicegrow(slicetyp, a, a.cap+(b.len-(a.cap-a.len)))
	}
	end := uintptr(unsafe.Pointer(a.array))
	end += uintptr(a.len) * slicetyp.elem.size
	copylen := slicetyp.elem.size * uintptr(b.len)
	memmove(unsafe.Pointer(end), unsafe.Pointer(b.array), copylen)
	a.len += b.len
	return a
}

// slicecopy takes a slice type and two slices, copying the second
// into the first, and returning the number of elements copied.
// The number of elements copied will be the minimum of the len of
// either slice.
func slicecopy(t unsafe.Pointer, a, b slice) uint {
	slicetyp := (*sliceType)(t)
	n := a.len
	if b.len < n {
		n = b.len
	}
	copylen := slicetyp.elem.size * uintptr(n)
	memmove(unsafe.Pointer(a.array), unsafe.Pointer(b.array), copylen)
	return n
}

func slicegrow(t *sliceType, a slice, newcap uint) slice {
	mem := malloc(uintptr(t.elem.size * uintptr(newcap)))
	if a.len > 0 {
		size := uintptr(a.len) * t.elem.size
		memcpy(mem, unsafe.Pointer(a.array), size)
	}
	return slice{(*uint8)(mem), a.len, newcap}
}

func sliceslice(t unsafe.Pointer, a slice, low, high int) slice {
	if high == -1 {
		high = int(a.len)
	} else {
		// TODO check upper bound
	}
	a.cap -= uint(low)
	a.len = uint(high - low)
	if low > 0 {
		slicetyp := (*sliceType)(t)
		newptr := uintptr(unsafe.Pointer(a.array))
		newptr += uintptr(slicetyp.elem.size * uintptr(low))
		a.array = (*uint8)(unsafe.Pointer(newptr))
	}
	return a
}
