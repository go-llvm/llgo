// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.
//
// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

const _NOSELGEN = 1

type SudoG struct {
	g           *G     // g and selgen constitute
	selgen      uint32 // a weak pointer to g
	link        *SudoG
	releasetime int64
	elem        unsafe.Pointer // data element
}

type CaseKind uint16

const (
	CaseRecv CaseKind = iota
	CaseSend
	CaseDefault
)

type Scase struct {
	sg        SudoG // must be first member (cast to Scase)
	chan_     *Hchan
	index     uint16
	kind      CaseKind
	receivedp *bool // pointer to received bool (recv2)
}

type Select struct {
	tcase      uint16   // total count of scase
	ncase      uint16   // currently filled scase
	index      uint16   // next index to allocate
	pollorder_ *uint16  // case poll order
	lockorder_ *uintptr // channel lock order
	scase      [1]Scase // one per case (in order of appearance)
}

type WaitQ struct {
	first *SudoG
	last  *SudoG
}

// removeg removes the current G from the WaitQ.
func (q *WaitQ) removeg() {
	g := myg()
	var prevsgp *SudoG
	l := &q.first
	for *l != nil {
		sgp := *l
		if sgp.g == g {
			*l = sgp.link
			if q.last == sgp {
				q.last = prevsgp
			}
			break
		}
		l, prevsgp = &sgp.link, sgp
	}
}

func (q *WaitQ) dequeue() *SudoG {
loop:
	sgp := q.first
	if sgp == nil {
		return nil
	}
	q.first = sgp.link

	// if sgp is stale, ignore it
	if sgp.selgen != _NOSELGEN &&
		(sgp.selgen != sgp.g.selgen ||
			!cas(&sgp.g.selgen, sgp.selgen, sgp.selgen+2)) {
		//println("INVALID PSEUDOG POINTER")
		goto loop
	}

	return sgp
}

func (q *WaitQ) enqueue(sgp *SudoG) {
	sgp.link = nil
	if q.first == nil {
		q.first = sgp
		q.last = sgp
		return
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

func makechan(t unsafe.Pointer, cap_ int) unsafe.Pointer {
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
	return mem
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
	return chansend((*chanType)(unsafe.Pointer(t)), c, val, nb)
}

func chansend(t *chanType, c_, ptr unsafe.Pointer, nb bool) bool {
	var mysg SudoG

	c := (*Hchan)(c_)
	if c == nil {
		if nb {
			return false
		}
		panic("unimplemented: send on nil chan should block forever")
	}

	c.lock.lock()

	if c.closed {
		goto closed
	}

	if c.dataqsiz > 0 {
		goto asynch
	}

	if sg := c.recvq.dequeue(); sg != nil {
		c.lock.unlock()
		gp := sg.g
		gp.param = unsafe.Pointer(sg)
		if sg.elem != nil {
			// TODO use copy alg
			memcpy(sg.elem, ptr, uintptr(c.elemsize))
		}
		gp.ready()
		return true
	}

	if nb {
		c.lock.unlock()
		return false
	}

	mysg.elem = ptr
	mysg.g = myg()
	mysg.g.param = nil
	mysg.selgen = _NOSELGEN
	c.sendq.enqueue(&mysg)
	c.lock.unlock()
	mysg.g.park("chan send")
	if mysg.g.param == nil {
		c.lock.lock()
		if !c.closed {
			panic("chansend: spurious wakeup")
		}
		goto closed
	}
	return true

asynch:
	if c.qcount >= c.dataqsiz {
		if nb {
			c.lock.unlock()
			return false
		}
		g := myg()
		mysg.g = g
		mysg.elem = nil
		mysg.selgen = _NOSELGEN
		c.sendq.enqueue(&mysg)
		c.lock.unlock()
		g.park("chan send")
		c.lock.lock()
		goto asynch
	}

	// TODO use copy alg
	memcpy(c.chanbuf(c.sendx), ptr, uintptr(c.elemsize))
	if c.sendx++; c.sendx == c.dataqsiz {
		c.sendx = 0
	}
	c.qcount++

	if sg := c.recvq.dequeue(); sg != nil {
		gp := sg.g
		c.lock.unlock()
		gp.ready()
	} else {
		c.lock.unlock()
	}
	return true

closed:
	c.lock.unlock()
	panic("send on closed channel")
}

// #llgo name: reflect.chanrecv
func reflect_chanrecv(t *rtype, c unsafe.Pointer, nb bool) (val unsafe.Pointer, selected, received bool) {
	// TODO
	return
}

func chanrecv(t *chanType, c_, ptr unsafe.Pointer, nb bool) (received bool) {
	c := (*Hchan)(c_)
	if c == nil {
		if nb {
			if ptr != nil {
				bzero(ptr, uintptr(c.elemsize))
			}
			return false
		}
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

	if sg := c.sendq.dequeue(); sg != nil {
		c.lock.unlock()
		if ptr != nil {
			// TODO use copy alg
			memcpy(ptr, sg.elem, uintptr(c.elemsize))
		}
		gp := sg.g
		gp.param = unsafe.Pointer(sg)
		gp.ready()
		return true
	}

	if nb {
		c.lock.unlock()
		if ptr != nil {
			bzero(ptr, uintptr(c.elemsize))
		}
		return false
	}

	mysg.elem = ptr
	mysg.g = myg()
	mysg.selgen = _NOSELGEN
	mysg.g.param = nil
	c.recvq.enqueue(&mysg)
	c.lock.unlock()
	mysg.g.park("chan receive")
	if mysg.g.param == nil {
		c.lock.lock()
		if !c.closed {
			panic("chanrecv: spurious wakeup")
		}
		goto closed
	}
	return true

asynch:
	if c.qcount <= 0 {
		if c.closed {
			goto closed
		}
		if nb {
			c.lock.unlock()
			if ptr != nil {
				bzero(ptr, uintptr(c.elemsize))
			}
			return false
		}
		g := myg()
		mysg.g = g
		mysg.elem = nil
		mysg.selgen = _NOSELGEN
		c.recvq.enqueue(&mysg)
		c.lock.unlock()
		g.park("chan receive")
		c.lock.lock()
		goto asynch
	}

	if ptr != nil {
		// TODO use copy alg
		memcpy(ptr, c.chanbuf(c.recvx), uintptr(c.elemsize))
	}
	bzero(c.chanbuf(c.recvx), uintptr(c.elemsize))
	if c.recvx++; c.recvx == c.dataqsiz {
		c.recvx = 0
	}
	c.qcount--

	if sg := c.sendq.dequeue(); sg != nil {
		gp := sg.g
		c.lock.unlock()
		gp.ready()
	} else {
		c.lock.unlock()
	}
	return true

closed:
	c.lock.unlock()
	if ptr != nil {
		bzero(ptr, uintptr(c.elemsize))
	}
	return false
}

// #llgo name: reflect.chanclose
func reflect_chanclose(c unsafe.Pointer) {
	chanclose(c)
}

func chanclose(c_ unsafe.Pointer) {
	c := (*Hchan)(c_)
	if c == nil {
		panic("close of nil channel")
	}

	c.lock.lock()
	if c.closed {
		c.lock.unlock()
		panic("close of closed channel")
	}

	c.closed = true

	// release all readers
	for {
		sg := c.recvq.dequeue()
		if sg == nil {
			break
		}
		sg.g.param = nil
		sg.g.ready()
	}

	// release all writers
	for {
		sg := c.sendq.dequeue()
		if sg == nil {
			break
		}
		sg.g.param = nil
		sg.g.ready()
	}

	c.lock.unlock()
}

func selectsize(size int32) uintptr {
	var n int32
	if size > 1 {
		n = size - 1
	}
	return unsafe.Sizeof(Select{}) +
		uintptr(n)*unsafe.Sizeof(Scase{}) +
		uintptr(size)*unsafe.Sizeof(&Hchan{}) +
		uintptr(size)*unsafe.Sizeof(uint16(0))
}

func selectinit(size int32, ptr unsafe.Pointer) {
	sel := (*Select)(ptr)
	sel.tcase = uint16(size)
	ptr = unsafe.Pointer(&sel.scase[0])
	ptr = unsafe.Pointer(uintptr(ptr) + uintptr(size)*unsafe.Sizeof(Scase{}))
	sel.lockorder_ = (*uintptr)(ptr)
	ptr = unsafe.Pointer(uintptr(ptr) + uintptr(size)*unsafe.Sizeof(*sel.lockorder_))
	sel.pollorder_ = (*uint16)(ptr)
}

func selectdefault(selectp_ unsafe.Pointer) {
	selectp := (*Select)(selectp_)
	i := selectp.ncase
	if i > selectp.tcase {
		panic("selectdefault: too many cases")
	}
	selectp.ncase++
	cas := &selectp.scase[i]
	cas.index = selectp.index
	selectp.index++
	cas.kind = CaseDefault
}

func selectsend(selectp_, ch, elem unsafe.Pointer) {
	selectp := (*Select)(selectp_)
	i := selectp.ncase
	if i > selectp.tcase {
		panic("selectsend: too many cases")
	}
	if ch == nil {
		selectp.index++
		return
	}
	selectp.ncase++
	cas := &selectp.scase[i]
	cas.index = selectp.index
	selectp.index++
	cas.kind = CaseSend
	cas.chan_ = (*Hchan)(ch)
	cas.sg.elem = elem
}

func selectrecv(selectp_, ch, elem unsafe.Pointer, received *bool) {
	selectp := (*Select)(selectp_)
	i := selectp.ncase
	if i > selectp.tcase {
		panic("selectrecv: too many cases")
	}
	if ch == nil {
		selectp.index++
		return
	}
	selectp.ncase++
	cas := &selectp.scase[i]
	cas.index = selectp.index
	selectp.index++
	cas.kind = CaseRecv
	cas.chan_ = (*Hchan)(ch)
	cas.sg.elem = elem
	cas.receivedp = received
}

func (s *Select) lockorder(i uint16) (ret uintptr) {
	ptr := uintptr(unsafe.Pointer(s.lockorder_))
	ptr += uintptr(i) * unsafe.Sizeof(ret)
	return *(*uintptr)(unsafe.Pointer(ptr))
}

func (s *Select) setlockorder(i uint16, val uintptr) {
	ptr := uintptr(unsafe.Pointer(s.lockorder_))
	ptr += uintptr(i) * unsafe.Sizeof(val)
	*(*uintptr)(unsafe.Pointer(ptr)) = val
}

func (s *Select) pollorder(i uint16) (ret uint16) {
	ptr := uintptr(unsafe.Pointer(s.pollorder_))
	ptr += uintptr(i) * unsafe.Sizeof(ret)
	return *(*uint16)(unsafe.Pointer(ptr))
}

func (s *Select) setpollorder(i, val uint16) {
	ptr := uintptr(unsafe.Pointer(s.pollorder_))
	ptr += uintptr(i) * unsafe.Sizeof(val)
	*(*uint16)(unsafe.Pointer(ptr)) = val
}

func (s *Select) lock() {
	var c uintptr
	for i := uint16(0); i < s.ncase; i++ {
		c0 := s.lockorder(i)
		if c0 != 0 && c0 != c {
			c = c0
			(*Hchan)(unsafe.Pointer(c)).lock.lock()
		}
	}
}

func (s *Select) unlock() {
	n := int32(s.ncase)
	r := int32(0)
	if n > 0 && s.lockorder(0) == 0 {
		// skip the default case
		r = 1
	}
	for i := int32(n - 1); i >= r; i-- {
		c := s.lockorder(uint16(i))
		if i > 0 && s.lockorder(uint16(i-1)) == c {
			continue
		}
		(*Hchan)(unsafe.Pointer(c)).lock.unlock()
	}
}

func selectgo(selectp_ unsafe.Pointer) int {
	sel := (*Select)(selectp_)
	for i := uint16(0); i < sel.ncase; i++ {
		sel.setpollorder(i, i)
	}
	// TODO shuffle poll order

	// sort the cases by Hchan address to get the locking order.
	// simple heap sort, to guarantee n log n time and constant stack footprint.
	for i := uint16(0); i < sel.ncase; i++ {
		j := i
		c := uintptr(unsafe.Pointer(sel.scase[j].chan_))
		for j > 0 {
			k := (j - 1) / 2
			if sel.lockorder(k) >= uintptr(unsafe.Pointer(c)) {
				break
			}
			sel.setlockorder(j, sel.lockorder(k))
			j = k
		}
		sel.setlockorder(j, c)
	}
	for i := uint16(sel.ncase); i > 0; {
		i--
		c := sel.lockorder(i)
		sel.setlockorder(i, sel.lockorder(0))
		j := uint16(0)
		for {
			k := j*2 + 1
			if k >= i {
				break
			}
			if k+1 < i && sel.lockorder(k) < sel.lockorder(k+1) {
				k++
			}
			if c < sel.lockorder(k) {
				sel.setlockorder(j, sel.lockorder(k))
				j = k
				continue
			}
			break
		}
		sel.setlockorder(j, c)
	}
	for i := uint16(0); i+1 < sel.ncase; i++ {
		if sel.lockorder(i) > sel.lockorder(i+1) {
			println("i=", i, sel.lockorder(i), sel.lockorder(i+1))
			panic("select: broken sort")
		}
	}
	sel.lock()

	// Common variables used in labeled sections.
	var cas *Scase
	var sg *SudoG
	var c *Hchan

loop:
	// pass 1 - look for something already waiting
	var defaultCase *Scase
	for i := uint16(0); i < sel.ncase; i++ {
		o := sel.pollorder(i)
		cas = &sel.scase[o]
		c = cas.chan_
		switch cas.kind {
		case CaseRecv:
			if c.dataqsiz > 0 {
				if c.qcount > 0 {
					goto asyncrecv
				}
			} else {
				if sg = c.sendq.dequeue(); sg != nil {
					goto syncrecv
				}
			}
			if c.closed {
				goto rclose
			}
		case CaseSend:
			if c.closed {
				goto sclose
			}
			if c.dataqsiz > 0 {
				if c.qcount < c.dataqsiz {
					goto asyncsend
				}
			} else {
				if sg = c.recvq.dequeue(); sg != nil {
					goto syncsend
				}
			}
		case CaseDefault:
			defaultCase = cas
		}
	}

	if defaultCase != nil {
		sel.unlock()
		cas = defaultCase
		goto retc
	}

	// pass 2 - enqueue on all chans
	for i := uint16(0); i < sel.ncase; i++ {
		o := sel.pollorder(i)
		cas = &sel.scase[o]
		c = cas.chan_
		sg = &cas.sg
		sg.g = myg()
		sg.selgen = sg.g.selgen
		switch cas.kind {
		case CaseRecv:
			c.recvq.enqueue(sg)
		case CaseSend:
			c.sendq.enqueue(sg)
		}
	}

	// New scope to avoid declaring jumped-over vars.
	{
		g := myg()
		g.param = nil
		sel.unlock()
		g.park("select")
		sel.lock()
		sg = (*SudoG)(g.param)
	}

	// pass 3 - dequeue from unsuccessful chans
	// otherwise they stack up on quiet channels
	for i := uint16(0); i < sel.ncase; i++ {
		cas := sel.scase[i]
		if &cas.sg != sg {
			c = cas.chan_
			if cas.kind == CaseSend {
				c.sendq.removeg()
			} else {
				c.recvq.removeg()
			}
		}
	}

	if sg == nil {
		goto loop
	}
	cas = (*Scase)(unsafe.Pointer(sg))
	c = cas.chan_

	if c.dataqsiz > 0 {
		panic("selectgo: shouldn't happen")
	}

	if cas.kind == CaseRecv && cas.receivedp != nil {
		*cas.receivedp = true
	}
	sel.unlock()
	goto retc

asyncrecv:
	// can receive from buffer
	if cas.receivedp != nil {
		*cas.receivedp = true
	}
	if cas.sg.elem != nil {
		// TODO use copy alg
		memcpy(cas.sg.elem, c.chanbuf(c.recvx), uintptr(c.elemsize))
	}
	bzero(c.chanbuf(c.recvx), uintptr(c.elemsize))
	if c.recvx++; c.recvx == c.dataqsiz {
		c.recvx = 0
	}
	c.qcount--
	if sg = c.sendq.dequeue(); sg != nil {
		gp := sg.g
		sel.unlock()
		gp.ready()
	} else {
		sel.unlock()
	}
	goto retc

asyncsend:
	// can send to buffer
	// TODO use copy alg
	memcpy(c.chanbuf(c.sendx), cas.sg.elem, uintptr(c.elemsize))
	if c.sendx++; c.sendx == c.dataqsiz {
		c.sendx = 0
	}
	c.qcount++
	if sg = c.recvq.dequeue(); sg != nil {
		gp := sg.g
		sel.unlock()
		gp.ready()
	} else {
		sel.unlock()
	}
	goto retc

syncrecv:
	// can receive from sleeping sender (sg)
	sel.unlock()
	if cas.receivedp != nil {
		*cas.receivedp = true
	}
	if cas.sg.elem != nil {
		// TODO use copy alg
		memcpy(cas.sg.elem, sg.elem, uintptr(c.elemsize))
	}
	sg.g.param = unsafe.Pointer(sg)
	sg.g.ready()
	goto retc

rclose:
	// read at end of closed channel
	sel.unlock()
	if cas.receivedp != nil {
		*cas.receivedp = false
	}
	if cas.sg.elem != nil {
		bzero(cas.sg.elem, uintptr(c.elemsize))
	}
	goto retc

syncsend:
	// can send to sleeping receiver (sg)
	sel.unlock()
	if sg.elem != nil {
		// TODO use copy alg
		memcpy(sg.elem, cas.sg.elem, uintptr(c.elemsize))
	}
	sg.g.param = unsafe.Pointer(sg)
	sg.g.ready()
	goto retc

retc:
	return int(cas.index) - 1

sclose:
	// send on closed channel
	sel.unlock()
	panic("send on closed channel")
}
