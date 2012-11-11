package main

import "unsafe"

type S struct {
	a int16
	b int32
	c int8
	d int64
}

func main() {
	var s S
	println(unsafe.Offsetof(s.a))
	println(unsafe.Offsetof(s.b))
	println(unsafe.Offsetof(s.c))
	println(unsafe.Offsetof(s.d))
}
