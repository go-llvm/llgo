// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

type G struct {
	parklock lock
}

func (g *G) park(reason string) {
	// TODO record reason
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
	}
	return tls_g
}
