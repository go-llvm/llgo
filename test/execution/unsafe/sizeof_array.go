// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

import "unsafe"

type uint24 struct {
	a uint16
	b uint8
}

func main() {
	var a [3]uint24
	println(unsafe.Sizeof(a))
}
