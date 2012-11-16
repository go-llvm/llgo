// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

type _chanItem struct {
	next  *_chanItem
	value uint8
}

// XXX dumb implementation for now, will make it
// a circular buffer later. If cap is zero, the
// buffer will point to an item on the receiver's
// stack.
type _chan struct {
	typ        *chanType
	head, tail *_chanItem
	cap        int
}

// #llgo name: reflect.makechan
func reflect_makechan(t *type_, cap_ uint32) unsafe.Pointer {
	return unsafe.Pointer(makechan(unsafe.Pointer(t), cap_))
}

func makechan(t unsafe.Pointer, cap_ uint32) *int8 {
	typ := (*type_)(t)
	c := new(_chan)
	c.typ = (*chanType)(unsafe.Pointer(&typ.commonType))
	c.cap = int(cap_)
	return (*int8)(unsafe.Pointer(c))
}

// #llgo name: reflect.chancap
func reflect_chancap(c unsafe.Pointer) int32 {
	return int32(chancap(c))
}

func chancap(c_ unsafe.Pointer) int {
	c := (*_chan)(c_)
	return c.cap
}

// #llgo name: reflect.chanlen
func reflect_chanlen(c unsafe.Pointer) int32 {
	return int32(chanlen(c))
}

func chanlen(c unsafe.Pointer) int {
	// TODO
	return 0
}

// #llgo name: reflect.chansend
func reflect_chansend(t *type_, c unsafe.Pointer, val unsafe.Pointer, nb bool) bool {
	// TODO
	return false
}

func chansend(c_, ptr unsafe.Pointer) {
	c := (*_chan)(c_)
	elemsize := c.typ.elem.size
	m := malloc(unsafe.Sizeof(_chanItem{}) + elemsize - 1)
	item := (*_chanItem)(m)
	if c.tail != nil {
		c.tail.next = item
	} else {
		c.head = item
	}
	c.tail = item
	memcpy(unsafe.Pointer(&item.value), ptr, elemsize)
}

// #llgo name: reflect.chanrecv
func reflect_chanrecv(t *type_, c unsafe.Pointer, nb bool) (val unsafe.Pointer, selected, received bool) {
	// TODO
	return
}

func chanrecv(c_, ptr unsafe.Pointer) {
	c := (*_chan)(c_)
	elemsize := c.typ.elem.size
	// TODO wait if channel is empty, panic on deadlock.
	if c.head != nil {
		item := c.head
		c.head = item.next
		memcpy(ptr, unsafe.Pointer(&item.value), elemsize)
	}
}

// #llgo name: reflect.chanclose
func reflect_chanclose(c unsafe.Pointer) {
	chanclose(c)
}

func chanclose(c_ unsafe.Pointer) {
	// TODO
}
