// Copyright 2014 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package cgo

import "unsafe"

// #llgo name: __go_byte_array_to_string
func go_byte_array_to_string(p unsafe.Pointer, n int) string {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = *(*byte)(unsafe.Pointer(uintptr(p) + uintptr(i)))
	}
	return string(buf )
}

// #llgo name: __go_string_to_byte_array
func go_string_to_byte_array(str string) []byte {
	return []byte(str)
}
