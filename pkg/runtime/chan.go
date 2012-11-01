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

func makechan(t unsafe.Pointer, cap_ int) *int8 {
	typ := (*type_)(t)
	c := new(_chan)
	c.typ = (*chanType)(unsafe.Pointer(&typ.commonType))
	c.cap = cap_
	return (*int8)(unsafe.Pointer(c))
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
