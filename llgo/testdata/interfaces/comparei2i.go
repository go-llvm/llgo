package main

import "unsafe"

func main() {
	var highbit uint32 = 1 << 31
	var pos0 float32 = 0
	var neg0 float32 = *(*float32)(unsafe.Pointer(&highbit))
	var i1 interface{} = pos0
	var i2 interface{} = neg0
	println(i1 == i2)
}
