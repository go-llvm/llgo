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

type slice struct {
	elements *int8
	length   int32
	capacity int32
}

// sliceappend takes a slice type and two slices, and returns the
// result of appending the second slice to the first, growing the
// slice as necessary.
func sliceappend(t unsafe.Pointer, a, b slice) slice {
	typ := (*type_)(t)
	slicetyp := (*sliceType)(unsafe.Pointer(&typ.commonType))
	if a.capacity-a.length < b.length {
		a = slicegrow(slicetyp, a, a.capacity+(b.length-(a.capacity-a.length)))
	}
	end := uintptr(unsafe.Pointer(a.elements))
	end += uintptr(a.length * slicetyp.elem.size)
	memmove(unsafe.Pointer(end), unsafe.Pointer(b.elements), int(slicetyp.elem.size*b.length))
	a.length += b.length
	return a
}

func slicegrow(t *sliceType, a slice, newcap int32) slice {
	mem := malloc(int(t.elem.size * newcap))
	if a.length > 0 {
		size := a.length * t.elem.size
		memcpy(mem, unsafe.Pointer(a.elements), int(size))
	}
	return slice{(*int8)(mem), a.length, newcap}
}

func sliceslice(t unsafe.Pointer, a slice, low, high int32) slice {
	if high == -1 {
		high = a.length
	} else {
		// TODO check upper bound
	}
	a.capacity -= low
	a.length = high - low
	if low > 0 {
		typ := (*type_)(t)
		slicetyp := (*sliceType)(unsafe.Pointer(&typ.commonType))
		newptr := uintptr(unsafe.Pointer(a.elements))
		newptr += uintptr(slicetyp.elem.size * low)
		a.elements = (*int8)(unsafe.Pointer(newptr))
	}
	return a
}
