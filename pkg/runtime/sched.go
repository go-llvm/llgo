// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

type G struct {
	parklock lock
	param    unsafe.Pointer
}

func (g *G) park(reason string) {
	// TODO record reason

	// When we initialise the G, we immediately
	// lock parklock. Thus, parklock can be thought
	// of as a binary semaphore.
	g.parklock.lock()
}

func (g *G) ready() {
	g.parklock.unlock()
}

// #llgo thread_local
var tls_g *G

func myg() *G {
	if tls_g == nil {
		tls_g = new(G)
		tls_g.parklock.lock()
	}
	return tls_g
}
