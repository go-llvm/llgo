// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

type __sigset_t [16]uint64

type __jmp_buf_tag struct {
	_ [8]int64
	_ int32
	_ [4]byte
	_ __sigset_t
}

type jmp_buf [1]__jmp_buf_tag
