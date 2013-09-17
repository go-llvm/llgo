// Copyright 2013 The llgo Authors.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// +build !freebsd,!linux,!pnacl

package runtime

type lock struct{}

func (l *lock) lock() {
	panic("not implemented")
}

func (l *lock) unlock() {
	panic("not implemented")
}
