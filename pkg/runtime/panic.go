// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

type deferred struct {
	f      func()
	caller uintptr
	next   *deferred
}

type panicstack struct {
	next  *panicstack
	value interface{}
}

func panic_(e interface{})
func caller_region(skip int32) uintptr
func pushdefer(f func())
func rundefers()
func current_panic() *panicstack

// #llgo attr: noinline
func callniladic(f func()) {
	// This exists just to avoid reproducing the
	// Go function calling logic in the C code.
	f()
}
