// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

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

// #llgo name: reflect.ismapkey
func reflect_ismapkey(t *rtype) bool {
	// TODO
	return false
}

// #llgo name: reflect.makemap
func reflect_makemap(t *map_) unsafe.Pointer {
	return makemap(unsafe.Pointer(t), 0)
}

func makemap(t unsafe.Pointer, cap int) unsafe.Pointer {
	m := (*map_)(malloc(uintptr(unsafe.Sizeof(map_{}))))
	return unsafe.Pointer(m)
}

// #llgo name: reflect.maplen
func reflect_maplen(m unsafe.Pointer) int32 {
	return int32(maplen(m))
}

func maplen(m unsafe.Pointer) int {
	if m != nil {
		return int((*map_)(m).length)
	}
	return 0
}

// #llgo name: reflect.mapassign
func reflect_mapassign(t *mapType, m, key, val unsafe.Pointer, ok bool) {
	if ok {
		ptr := maplookup(unsafe.Pointer(t), m, key, true)
		// TODO use copy alg
		memmove(ptr, val, t.elem.size)
	} else {
		mapdelete(unsafe.Pointer(t), m, key)
	}
}

// #llgo name: reflect.mapaccess
func reflect_mapaccess(t *rtype, m, key unsafe.Pointer) (val unsafe.Pointer, ok bool) {
	ptr := maplookup(unsafe.Pointer(t), m, key, false)
	return ptr, ptr != nil
}

// mapaccess copies the value for the given key out if that key exists,
// else nil. The return value is true iff the key exists.
func mapaccess(t unsafe.Pointer, m_, key, outval unsafe.Pointer) bool {
	maptyp := (*mapType)(t)
	elemsize := uintptr(maptyp.elem.size)
	ptr := maplookup(t, m_, key, false)
	if ptr != nil {
		memcpy(outval, ptr, elemsize)
		return true
	}
	bzero(outval, elemsize)
	return false
}

// maplookup returns a pointer to the value for the given key
func maplookup(t unsafe.Pointer, m_, key unsafe.Pointer, insert bool) unsafe.Pointer {
	if m_ == nil {
		return nil
	}
	m := (*map_)(m_)

	maptyp := (*mapType)(t)
	ptrsize := uintptr(unsafe.Sizeof(m.head.next))
	keysize := uintptr(maptyp.key.size)
	keyoffset := align(ptrsize, uintptr(maptyp.key.align))
	elemsize := uintptr(maptyp.elem.size)
	elemoffset := align(keyoffset+keysize, uintptr(maptyp.elem.align))
	entrysize := elemoffset + elemsize

	// Search for the entry with the specified key.
	keyalgs := unsafe.Pointer(maptyp.key.alg)
	keyeqptr := unsafe.Pointer(uintptr(keyalgs) + unsafe.Sizeof(maptyp.key.alg))
	keyeqfun := *(*unsafe.Pointer)(keyeqptr)
	var last *mapentry
	for ptr := m.head; ptr != nil; ptr = ptr.next {
		keyptr := unsafe.Pointer(uintptr(unsafe.Pointer(ptr)) + keyoffset)
		if eqalg(keyeqfun, keysize, key, keyptr) {
			elemptr := unsafe.Pointer(uintptr(unsafe.Pointer(ptr)) + elemoffset)
			return elemptr
		}
		last = ptr
	}

	// Not found: insert the key if requested.
	if insert {
		newentry := (*mapentry)(malloc(entrysize))
		newentry.next = nil
		keyptr := unsafe.Pointer(uintptr(unsafe.Pointer(newentry)) + keyoffset)
		elemptr := unsafe.Pointer(uintptr(unsafe.Pointer(newentry)) + elemoffset)
		memcpy(keyptr, key, keysize)
		if last != nil {
			last.next = newentry
		} else {
			m.head = newentry
		}
		m.length++
		return elemptr
	}

	return nil
}

func mapdelete(t unsafe.Pointer, m_, key unsafe.Pointer) {
	if m_ == nil {
		return
	}
	m := (*map_)(m_)

	maptyp := (*mapType)(t)
	ptrsize := uintptr(unsafe.Sizeof(m.head.next))
	keysize := uintptr(maptyp.key.size)
	keyoffset := align(ptrsize, uintptr(maptyp.key.align))

	// Search for the entry with the specified key.
	keyalgs := unsafe.Pointer(maptyp.key.alg)
	keyeqptr := unsafe.Pointer(uintptr(keyalgs) + unsafe.Sizeof(maptyp.key.alg))
	keyeqfun := *(*unsafe.Pointer)(keyeqptr)
	var last *mapentry
	for ptr := m.head; ptr != nil; ptr = ptr.next {
		keyptr := unsafe.Pointer(uintptr(unsafe.Pointer(ptr)) + keyoffset)
		if eqalg(keyeqfun, keysize, key, keyptr) {
			if last == nil {
				m.head = ptr.next
			} else {
				last.next = ptr.next
			}
			free(unsafe.Pointer(ptr))
			m.length--
			return
		}
		last = ptr
	}
}

// #llgo name: reflect.mapiterinit
func reflect_mapiterinit(t *rtype, m_ unsafe.Pointer) *byte {
	// TODO
	return nil
}

// #llgo name: reflect.mapiterkey
func reflect_mapiterkey(it *byte) (key unsafe.Pointer, ok bool) {
	// TODO
	return
}

// #llgo name: reflect.mapiternext
func reflect_mapiternext(it *byte) {
	// TODO
}

func mapnext(t unsafe.Pointer, m *map_, nextin unsafe.Pointer) (nextout, pk, pv unsafe.Pointer) {
	if m == nil {
		return
	}
	ptr := (*mapentry)(nextin)
	if ptr == nil {
		ptr = m.head
	} else {
		ptr = ptr.next
	}
	if ptr != nil {
		maptyp := (*mapType)(t)
		ptrsize := uintptr(unsafe.Sizeof(m.head.next))
		keysize := uintptr(maptyp.key.size)
		keyoffset := align(ptrsize, uintptr(maptyp.key.align))
		elemoffset := align(keyoffset+keysize, uintptr(maptyp.elem.align))
		nextout = unsafe.Pointer(ptr)
		pk = unsafe.Pointer(uintptr(unsafe.Pointer(ptr)) + keyoffset)
		pv = unsafe.Pointer(uintptr(unsafe.Pointer(ptr)) + elemoffset)
	}
	return
}
