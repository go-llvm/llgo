// Copyright 2014 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package syscall

func Cgocall()     {}
func CgocallDone() {}

// defined in cgo.c

func GetErrno() Errno
func SetErrno(n Errno)

