// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

type deferred struct {
	f    func()
	next *deferred
}

func panic_(e interface{}) {
	print("panic(")
	printany(e)
	println(")")
	llvm_trap()
}

func rundefers(d *deferred) {
	for ; d != nil; d = d.next {
		d.f()
	}
}

func pushdefer(f func(), top **deferred) {
	d := new(deferred)
	d.f = f
	d.next = *top
	*top = d
}
