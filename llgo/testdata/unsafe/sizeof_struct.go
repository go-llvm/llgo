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
