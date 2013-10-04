// Copyright 2013 The llgo Authors.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// +build !freebsd,!linux,!pnacl

package runtime

type lock uint32

func (l *lock) lock() {
	for !cas((*uint32)(l), 0, 1) {
	}
}

func (l *lock) unlock() {
	xchg((*uint32)(l), 0)
}
