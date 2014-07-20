// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

import "unsafe"

const ptrSize = unsafe.Sizeof((*byte)(nil))

var x [ptrSize]int

func main() {
	println(ptrSize)
	println(len(x))
}
