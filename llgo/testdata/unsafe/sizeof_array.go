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
