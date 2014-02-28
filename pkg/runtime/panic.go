// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

type defers struct {
	j      jmp_buf
	caller uintptr
	d      *deferred
	next   *defers
}

type deferred struct {
	f    func()
	next *deferred
}

type panicstack struct {
	next  *panicstack
	value interface{}
}

func panic_(e interface{})
func caller_region(skip int32) uintptr
func pushdefer(f func())
func initdefers(*defers)
func rundefers()
func current_panic() *panicstack
func pop_panic()
func recover_(int32) interface{}

// #llgo name: llvm.setjmp
func llvm_setjmp(*int8) int32

// #llgo attr: noinline
func callniladic(f func()) {
	// This exists just to avoid reproducing the
	// Go function calling logic in the C code.
	f()
}

// #llgo name: runtime_throw
// #llgo attr: noreturn
func throw(cstr *byte) {
	str := _string{cstr, int(c_strlen(cstr))}
	panic_(*(*string)(unsafe.Pointer(&str)))
}

// guardedcall0 calls f, swallowing and panics.
func guardedcall0(f func())

// guardedcall1 calls f, calling errback if a panic occurs.
func guardedcall1(f func(), errback func())
