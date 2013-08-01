// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

type SudoG struct {
	g *G // g and selgen constitute
	//selgen      int32 // a weak pointer to g
	link        *SudoG
	releasetime int64
	elem        *byte // data element
}

type WaitQ struct {
	first *SudoG
	last  *SudoG
}

func (q *WaitQ) dequeue() *SudoG {
	sgp := q.first
	if sgp == nil {
		return nil
	}
	q.first = sgp.link
	return sgp
}

func (q *WaitQ) enqueue(sgp *SudoG) {
	sgp.link = nil
	if q.first == nil {
		q.first = sgp
		q.last = sgp
	}
	q.last.link = sgp
	q.last = sgp
}

type Hchan struct {
	qcount    uint // total data in the q
	dataqsiz  uint // size of the circular q
	elemsize  uint16
	closed    bool
	elemalign uint8
	//Alg       *elemalg // interface for element type
	sendx uint  // send index
	recvx uint  // receive index
	recvq WaitQ // list of recv waiters
	sendq WaitQ // list of send waiters
	lock
}

// Buffer follows Hchan immediately in memory.
// chanbuf(c, i) is pointer to the i'th slot in the buffer.
//#define chanbuf(c, i) ((byte*)((c)+1)+(uintptr)(c)->elemsize*(i))
func (c *Hchan) chanbuf(i uint) unsafe.Pointer {
	ptr := uintptr(unsafe.Pointer(c))
	ptr += unsafe.Sizeof(*c)
	ptr = align(ptr, uintptr(c.elemalign))
	ptr += uintptr(c.elemsize) * uintptr(i)
	return unsafe.Pointer(ptr)
}

// #llgo name: reflect.makechan
func reflect_makechan(t *rtype, cap_ int) unsafe.Pointer {
	return unsafe.Pointer(makechan(unsafe.Pointer(t), cap_))
}

func makechan(t unsafe.Pointer, cap_ int) *int8 {
	typ := (*chanType)(t)
	size := unsafe.Sizeof(Hchan{})
	if cap_ > 0 {
		size = align(size, uintptr(typ.elem.align))
		size += uintptr(typ.elem.size) * uintptr(cap_)
	}
	mem := malloc(size)
	c := (*Hchan)(mem)
	c.elemsize = uint16(typ.elem.size)
	c.elemalign = uint8(typ.elem.align)
	c.dataqsiz = uint(cap_)
	return (*int8)(mem)
}

// #llgo name: reflect.chancap
func reflect_chancap(c unsafe.Pointer) int32 {
	return int32(chancap(c))
}

func chancap(c_ unsafe.Pointer) int {
	c := (*Hchan)(c_)
	if c == nil {
		return 0
	}
	return int(c.dataqsiz)
}

// #llgo name: reflect.chanlen
func reflect_chanlen(c unsafe.Pointer) int32 {
	return int32(chanlen(c))
}

func chanlen(c_ unsafe.Pointer) int {
	c := (*Hchan)(c_)
	if c == nil {
		return 0
	}
	return int(c.qcount)
}

// #llgo name: reflect.chansend
func reflect_chansend(t *rtype, c unsafe.Pointer, val unsafe.Pointer, nb bool) bool {
	// TODO
	return false
}

func chansend(t *chanType, c_, ptr unsafe.Pointer) {
	var mysg SudoG

	c := (*Hchan)(c_)
	if c == nil {
		panic("unimplemented: send on nil chan should block forever")
	}

	c.lock.lock()

	if c.closed {
		goto closed
	}

	if c.dataqsiz > 0 {
		goto asynch
	}

	panic("synch send unimplemented")

asynch:
	if c.qcount >= c.dataqsiz {
		g := myg()
		mysg.g = g
		mysg.elem = nil
		c.sendq.enqueue(&mysg)
		c.unlock()
		g.park("chan send")
		c.lock.lock()
		goto asynch
	}

	// TODO use copy alg
	memcpy(c.chanbuf(c.sendx), ptr, uintptr(c.elemsize))
	c.sendx++
	if c.sendx == c.dataqsiz {
		c.sendx = 0
	}
	c.qcount++

	sg := c.recvq.dequeue()
	if sg != nil {
		gp := sg.g
		c.unlock()
		gp.ready()
	} else {
		c.unlock()
	}
	return

closed:
	c.unlock()
	panic("send on closed channel")
}

// #llgo name: reflect.chanrecv
func reflect_chanrecv(t *rtype, c unsafe.Pointer, nb bool) (val unsafe.Pointer, selected, received bool) {
	// TODO
	return
}

func chanrecv(t *chanType, c_, ptr unsafe.Pointer) (received bool) {
	c := (*Hchan)(c_)
	if c == nil {
		myg().park("chan receive (nil chan)")
	}

	c.lock.lock()
	var mysg SudoG

	if c.dataqsiz > 0 {
		goto asynch
	}

	if c.closed {
		goto closed
	}

	panic("synch receive not implemented")

asynch:
	if c.qcount <= 0 {
		if c.closed {
			goto closed
		}
		g := myg()
		mysg.g = g
		mysg.elem = nil
		c.recvq.enqueue(&mysg)
		c.unlock()
		g.park("chan receive")
		c.lock.lock()
		goto asynch
	}

	if uintptr(ptr) != 0 {
		// TODO use copy alg
		memcpy(ptr, c.chanbuf(c.recvx), uintptr(c.elemsize))
	}
	bzero(c.chanbuf(c.recvx), uintptr(c.elemsize))
	c.recvx++
	if c.recvx == c.dataqsiz {
		c.recvx = 0
	}
	c.qcount--

	sg := c.sendq.dequeue()
	if sg != nil {
		gp := sg.g
		c.unlock()
		gp.ready()
	} else {
		c.unlock()
	}
	return true

closed:
	panic("unimplemented: receive on closed chan")
	return false
}

// #llgo name: reflect.chanclose
func reflect_chanclose(c unsafe.Pointer) {
	chanclose(c)
}

func chanclose(c_ unsafe.Pointer) {
	// TODO
	panic("unimplemented: chanclose")
}
