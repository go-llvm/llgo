/*
Copyright (c) 2011, 2012 Andrew Wilkins <axwalk@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package runtime

import "unsafe"

type _string struct {
	str *uint8
	len int
}

func strcat(a, b _string) _string {
	if a.len == 0 {
		return b
	} else if b.len == 0 {
		return a
	}

	mem := malloc(a.len + b.len)
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

// vim: set ft=go:
