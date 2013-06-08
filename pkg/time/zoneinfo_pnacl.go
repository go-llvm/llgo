// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build pnacl

package time

import (
	"errors"
)

func initLocal() {
	// TODO get timezone from browser?
	localLoc.name = "UTC"
}

func loadLocation(name string) (*Location, error) {
	// TODO
	return nil, errors.New("unknown time zone " + name)
}
