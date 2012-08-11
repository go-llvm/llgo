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

type map_ struct {
	length int32
	head   *mapentry
}

type mapentry struct {
	next *mapentry
	// after this comes the key, then the value.
}

func maplookup(t unsafe.Pointer, m *map_, key unsafe.Pointer, insert bool) unsafe.Pointer {
	if m == nil {
		return 0
	}

	typ := (*type_)(t)
	maptyp := (*mapType)(unsafe.Pointer(&typ.commonType))

	// Calculate sizes, offsets.
	ptrsize := uintptr(unsafe.Sizeof(m.head.next))
	keysize := uintptr(maptyp.key.size)
	keyalign := uintptr(maptyp.key.align)
	keyoffset := ptrsize
	if keyoffset%keyalign != 0 {
		keyoffset += keyalign - (keyoffset % keyalign)
	}
	elemsize := uintptr(maptyp.elem.size)
	elemalign := uintptr(maptyp.elem.align)
	elemoffset := keyoffset + keysize
	if elemoffset%elemalign != 0 {
		elemoffset += elemalign - (elemoffset % elemalign)
	}
	entrysize := elemoffset + elemsize

	// Search for the entry with the specified key.
	keyalgs := unsafe.Pointer(maptyp.key.alg)
	keyeqptr := unsafe.Pointer(uintptr(keyalgs) + unsafe.Sizeof(*maptyp.key.alg))
	keyeqfun := *(*equalalg)(keyeqptr)
	var last *mapentry
	for ptr := m.head; ptr != nil; ptr = ptr.next {
		last = ptr
		keyptr := unsafe.Pointer(uintptr(unsafe.Pointer(ptr)) + keyoffset)
		if keyeqfun(keysize, key, keyptr) {
			elemptr := unsafe.Pointer(uintptr(unsafe.Pointer(ptr)) + elemoffset)
			return elemptr
		}
	}

	// Not found: insert the key if requested.
	if insert {
		newentry := (*mapentry)(malloc(entrysize))
		keyptr := unsafe.Pointer(uintptr(unsafe.Pointer(newentry) + keyoffset))
		elemptr := unsafe.Pointer(uintptr(unsafe.Pointer(newentry)) + elemoffset)
		memcpy(keyptr, key, keysize)
		if last != nil {
			last.next = newentry
		} else {
			m.head = newentry
		}
		return elemptr
	}

	return 0
}
