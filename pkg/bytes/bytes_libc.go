// Copyright 2013 The llgo Authors
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package bytes

import "unsafe"

// #llgo name: memchr
func memchr(s *byte, c byte, n int) uintptr

// #llgo name: memcmp
func memcmp(a, b *byte, n int) int32

// #llgo name: bytes.IndexByte
func indexByte(s []byte, c byte) int {
    n := len(s)
    if n > 0 {
        ptr := memchr(&s[0], c, n)
        if ptr != 0 {
            diff := ptr - uintptr(unsafe.Pointer(&s[0]))
            return int(diff)
        }
    }
    return -1
}

// #llgo name: bytes.Equal
func equal(a, b []byte) bool {
    if n := len(a); len(b) == n {
        return n == 0 || memcmp(&a[0], &b[0], n) == 0
    }
    return false
}

