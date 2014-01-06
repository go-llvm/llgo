// Copyright 2014 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// +build ignore

// This is used by llgo-dist to feed into cgo.

package runtime

// #include <setjmp.h>
import "C"

// Create aliases for any C types we need in Go code (or the frontend).
type jmp_buf C.jmp_buf

