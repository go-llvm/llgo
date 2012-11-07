package main

import "unsafe"

const ptrSize = unsafe.Sizeof((*byte)(nil))

func main() {
	println(ptrSize)
}
