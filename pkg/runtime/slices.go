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

// sliceappend takes a slice type and two slices, and returns the
// result of appending the second slice to the first, growing the
// slice as necessary.
func sliceappend(t unsafe.Pointer, a, b slice) slice {
	typ := (*type_)(t)
	slicetyp := (*sliceType)(unsafe.Pointer(&typ.commonType))
	if a.cap-a.len < b.len {
		a = slicegrow(slicetyp, a, a.cap+(b.len-(a.cap-a.len)))
	}
	end := uintptr(unsafe.Pointer(a.array))
	end += uintptr(a.len) * slicetyp.elem.size
	copylen := int(slicetyp.elem.size * uintptr(b.len))
	memmove(unsafe.Pointer(end), unsafe.Pointer(b.array), copylen)
	a.len += b.len
	return a
}

// slicecopy takes a slice type and two slices, copying the second
// into the first, and returning the number of elements copied.
// The number of elements copied will be the minimum of the len of
// either slice.
func slicecopy(t unsafe.Pointer, a, b slice) uint {
	typ := (*type_)(t)
	slicetyp := (*sliceType)(unsafe.Pointer(&typ.commonType))
	n := a.len
	if b.len < n {
		n = b.len
	}
	copylen := int(slicetyp.elem.size * uintptr(n))
	memmove(unsafe.Pointer(a.array), unsafe.Pointer(b.array), copylen)
	return n
}

func slicegrow(t *sliceType, a slice, newcap uint) slice {
	mem := malloc(uintptr(t.elem.size * uintptr(newcap)))
	if a.len > 0 {
		size := uintptr(a.len) * t.elem.size
		memcpy(mem, unsafe.Pointer(a.array), int(size))
	}
	return slice{(*uint8)(mem), a.len, newcap}
}

func sliceslice(t unsafe.Pointer, a slice, low, high int32) slice {
	if high == -1 {
		high = a.len
	} else {
		// TODO check upper bound
	}
	a.cap -= low
	a.len = high - low
	if low > 0 {
		typ := (*type_)(t)
		slicetyp := (*sliceType)(unsafe.Pointer(&typ.commonType))
		newptr := uintptr(unsafe.Pointer(a.array))
		newptr += uintptr(slicetyp.elem.size * uintptr(low))
		a.array = (*uint8)(unsafe.Pointer(newptr))
	}
	return a
}
