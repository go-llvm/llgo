// Copyright 2013 The llgo Authors
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package strings

// #llgo name: strings.IndexByte
func indexByte(s string, c byte) int {
	// TODO: optimize
	for i, r := range s {
		if byte(r) == c {
			return i
		}
	}
	return -1
}
