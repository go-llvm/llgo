// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

import "unsafe"

type a struct {
	a int16
	b int32
	c int8
	d int64
}

func main() {
	println(unsafe.Sizeof(a{}))
}
