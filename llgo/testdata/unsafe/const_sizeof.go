package main

import "unsafe"

const ptrSize = unsafe.Sizeof((*byte)(nil))
var x [ptrSize]int

func main() {
	println(ptrSize)
	println(len(x))
}
