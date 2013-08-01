// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.
//
// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build freebsd linux

package runtime

import (
	"unsafe"
)

// This implementation depends on OS-specific implementations of
//
//	runtime·futexsleep(uint32 *addr, uint32 val, int64 ns)
//		Atomically,
//			if(*addr == val) sleep
//		Might be woken up spuriously; that's allowed.
//		Don't sleep longer than ns; ns < 0 means forever.
//
//	runtime·futexwakeup(uint32 *addr, uint32 cnt)
//		If any procs are sleeping on addr, wake up at most cnt.

const (
	_MUTEX_UNLOCKED = iota
	_MUTEX_LOCKED
	_MUTEX_SLEEPING
)

const (
	_ACTIVE_SPIN     = 4
	_ACTIVE_SPIN_CNT = 30
	_PASSIVE_SPIN    = 1
)

type lock struct {
	key uintptr
}

// TODO
const ncpu = 1

// TODO
func procyield(n int) {}

func osyield()

// Possible lock states are MUTEX_UNLOCKED, MUTEX_LOCKED and MUTEX_SLEEPING.
// MUTEX_SLEEPING means that there is presumably at least one sleeping thread.
// Note that there can be spinning threads during all states - they do not
// affect mutex's state.
func (l *lock) lock() {
	var i, v, wait uint32

	//if(m->locks++ < 0)
	//	runtime·throw("runtime·lock: lock count");

	// Speculative grab for lock.
	v = xchg((*uint32)(unsafe.Pointer(&l.key)), _MUTEX_LOCKED)
	if v == _MUTEX_UNLOCKED {
		return
	}

	// wait is either MUTEX_LOCKED or MUTEX_SLEEPING
	// depending on whether there is a thread sleeping
	// on this mutex.  If we ever change l->key from
	// MUTEX_SLEEPING to some other value, we must be
	// careful to change it back to MUTEX_SLEEPING before
	// returning, to ensure that the sleeping thread gets
	// its wakeup call.
	wait = v

	// On uniprocessor's, no point spinning.
	// On multiprocessors, spin for ACTIVE_SPIN attempts.
	var spin int
	if ncpu > 1 {
		spin = _ACTIVE_SPIN
	}

	for {
		// Try for lock, spinning.
		for i := 0; i < spin; i++ {
			for l.key == _MUTEX_UNLOCKED {
				if cas((*uint32)(unsafe.Pointer(&l.key)), _MUTEX_UNLOCKED, wait) {
					return
				}
			}
			procyield(_ACTIVE_SPIN_CNT)
		}

		// Try for lock, rescheduling.
		for i := 0; i < _PASSIVE_SPIN; i++ {
			for l.key == _MUTEX_UNLOCKED {
				if cas((*uint32)(unsafe.Pointer(&l.key)), _MUTEX_UNLOCKED, wait) {
					return
				}
			}
			osyield()
		}

		// Sleep.
		v = xchg((*uint32)(unsafe.Pointer(&l.key)), _MUTEX_SLEEPING)
		if v == _MUTEX_UNLOCKED {
			return
		}
		wait = _MUTEX_SLEEPING
		futexsleep((*uint32)(unsafe.Pointer(&l.key)), _MUTEX_SLEEPING, -1)
	}
}

func (l *lock) unlock() {
	v := xchg((*uint32)(unsafe.Pointer(&l.key)), _MUTEX_UNLOCKED)
	if v == _MUTEX_UNLOCKED {
		panic("unlock of unlocked lock")
	}
	if v == _MUTEX_SLEEPING {
		futexwakeup((*uint32)(unsafe.Pointer(&l.key)), 1)
	}
}

/*
// One-time notifications.
void
runtime·noteclear(Note *n)
{
	n->key = 0;
}

void
runtime·notewakeup(Note *n)
{
	if(runtime·xchg((*uint32)&n->key, 1))
		runtime·throw("notewakeup - double wakeup");
	runtime·futexwakeup((*uint32)&n->key, 1);
}

void
runtime·notesleep(Note *n)
{
	if(m->profilehz > 0)
		runtime·setprof(false);
	while(runtime·atomicload((*uint32)&n->key) == 0)
		runtime·futexsleep((*uint32)&n->key, 0, -1);
	if(m->profilehz > 0)
		runtime·setprof(true);
}

void
runtime·notetsleep(Note *n, int64 ns)
{
	int64 deadline, now;

	if(ns < 0) {
		runtime·notesleep(n);
		return;
	}

	if(runtime·atomicload((*uint32)&n->key) != 0)
		return;

	if(m->profilehz > 0)
		runtime·setprof(false);
	deadline = runtime·nanotime() + ns;
	for(;;) {
		runtime·futexsleep((*uint32)&n->key, 0, ns);
		if(runtime·atomicload((*uint32)&n->key) != 0)
			break;
		now = runtime·nanotime();
		if(now >= deadline)
			break;
		ns = deadline - now;
	}
	if(m->profilehz > 0)
		runtime·setprof(true);
}
*/
