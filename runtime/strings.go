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

type str struct {
	ptr *uint8
	size int
}

func malloc(int) unsafe.Pointer
func memcpy(dst, src unsafe.Pointer, size int)

func strcat(a, b str) str {
	if a.size == 0 {
		return b
	} else if b.size == 0 {
		return a
	}

	mem := malloc(a.size + b.size)
	if mem == unsafe.Pointer(uintptr(0)) {
		// TODO panic? abort?
	}

	memcpy(mem, unsafe.Pointer(a.ptr), a.size)
	memcpy(unsafe.Pointer(uintptr(mem) + uintptr(a.size)),
		   unsafe.Pointer(b.ptr), b.size)

	a.ptr = (*uint8)(mem)
	a.size = a.size + b.size
	return a
}

// vim: set ft=go:

