// Copyright 2011 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

// #llgo name: strlen
func c_strlen(*uint8) uintptr

func strcat(a, b _string) _string {
	if a.len == 0 {
		return b
	} else if b.len == 0 {
		return a
	}

	mem := malloc(uintptr(a.len + b.len))
	if mem == unsafe.Pointer(uintptr(0)) {
		// TODO panic? abort?
	}

	memcpy(mem, unsafe.Pointer(a.str), a.len)
	memcpy(unsafe.Pointer(uintptr(mem)+uintptr(a.len)), unsafe.Pointer(b.str), b.len)

	a.str = (*uint8)(mem)
	a.len = a.len + b.len
	return a
}

func strcmp(a, b _string) int32 {
	sz := a.len
	if b.len < sz {
		sz = b.len
	}
	aptr, bptr := a.str, b.str
	for i := 0; i < sz; i++ {
		c1, c2 := *aptr, *bptr
		switch {
		case c1 < c2:
			return -1
		case c1 > c2:
			return 1
		}
		aptr = (*uint8)(unsafe.Pointer((uintptr(unsafe.Pointer(aptr)) + 1)))
		bptr = (*uint8)(unsafe.Pointer((uintptr(unsafe.Pointer(bptr)) + 1)))
	}
	switch {
	case a.len < b.len:
		return -1
	case a.len > b.len:
		return 1
	}
	return 0
}

func stringslice(a _string, low, high int32) _string {
	if high == -1 {
		high = a.len
	} else {
		// TODO check upper bound
	}
	if low > 0 {
		newptr := uintptr(unsafe.Pointer(a.str))
		newptr += uintptr(low)
		a.str = (*uint8)(unsafe.Pointer(newptr))
	}
	a.len = high - low
	return a
}

// strnext returns the rune/codepoint at position i,
// along with the number of bytes that make it up.
func strnext(s _string, i int) (n int, value rune) {
	ptr := uintptr(unsafe.Pointer(s.str)) + uintptr(i)
	c0 := *(*int8)(unsafe.Pointer(ptr))
	n = uint(ctlz8(^c0))
	if n == 0 {
		value = rune(c0)
		n = 1
		return
	} else if i+n > s.len {
		value = 0xFFFD
		n = 1
		return
	}
	value = rune(c0 & int8(0xFF>>n))
	for j := uint(1); j < n; j++ {
		c := *(*int8)(unsafe.Pointer(ptr) + uintptr(j))
		// Make sure only the top bit is set.
		if c&0xC0 != 0x80 {
			n = 1
			value = 0xFFFD
			return
		}
		// only take the low 6 bits of continuation bytes.
		value = (value << 6) | rune(c&0x3F)
	}
	return
}

// strrune converts a rune to a string.
func strrune(n int64) _string {
	var b [4]uint8
	if n >= 0x10000 {
		// TODO panic if range > 0x1fffff?
		b[0] = 0xf0 | uint8((n>>18)&0x3f)
		b[1] = 0x80 | uint8((n>>12)&0x3f)
		b[2] = 0x80 | uint8((n>>6)&0x3f)
		b[3] = 0x80 | uint8(n&0x3f)
		return _string{&b[0], 4}
	}
	if n >= 0x800 {
		b[0] = 0xe0 | uint8((n>>12)&0x3f)
		b[1] = 0x80 | uint8((n>>6)&0x3f)
		b[2] = 0x80 | uint8(n&0x3f)
		return _string{&b[0], 3}
	}
	if n >= 0x80 {
		b[0] = 0xc0 | uint8((n>>6)&0x3f)
		b[1] = 0x80 | uint8(n&0x3f)
		return _string{&b[0], 2}
	}
	b[0] = uint8(n)
	return _string{&b[0], 1}
}

// vim: set ft=go:
