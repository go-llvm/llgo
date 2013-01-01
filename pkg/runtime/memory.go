/*
Copyright (c) 2012 Andrew Wilkins <axwalk@gmail.com>

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

func bzero(dst unsafe.Pointer, size uintptr) {
	memset(dst, 0, size)
}

// #llgo name: mmap
func mmap(addr unsafe.Pointer, len uintptr, prot int32, flags int32, fd int32, off uintptr) unsafe.Pointer

func align(p uintptr, align uintptr) uintptr {
	if p%align != 0 {
		p += (align - (p % align))
	}
	return p
}

// memalign allocates at least size bytes of memory,
// aligned to the specified alignment.
//
// XXX the returned value currently may not be freed.
// Once we have a garbage collector, this will not be
// a problem.
func memalign(align_ uintptr, size uintptr) unsafe.Pointer {
	if align_ > 1 {
		size += align_
	}

	const (
		PROT_READ   = 0x1
		PROT_WRITE  = 0x2
		PROT_EXEC   = 0x4
		MAP_PRIVATE = 0x02
		MAP_ANON    = 0x20
	)

	// XXX this is a hack so we get executable memory for closures.
	const prot = PROT_READ | PROT_WRITE | PROT_EXEC
	const flags = MAP_ANON | MAP_PRIVATE
	p := mmap(nil, size, prot, flags, -1, 0)
	if p == unsafe.Pointer(uintptr((1<<32)-1)) {
		panic("mmap failed")
	}
	return unsafe.Pointer(align(uintptr(p), align_))
}
