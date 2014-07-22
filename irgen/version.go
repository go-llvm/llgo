// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package irgen

const (
	goVersion = "go1.3"
	version   = "0.1"
)

// GoVersion returns the version of Go that we are targeting.
func GoVersion() string {
	return goVersion
}

// Version returns the version of the llgo compiler.
func Version() string {
	return version
}
