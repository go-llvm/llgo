// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build pnacl

package os

func hostname() (name string, err error) {
	// XXX Native Client has some experimental sockets stuff,
    // so we may be able to get the hostname in the future.
	return "localhost", nil
}
