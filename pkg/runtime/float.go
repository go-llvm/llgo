// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

func isNaN(v float64) bool {
	return v != v
}

// #llgo linkage: available_externally
var nan, posinf, neginf float64
