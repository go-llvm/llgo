package main

func testShrUint32() {
	var u uint32 = 0xFFFFFFFF
	println(u >> 1) // should be zero-filled
}

func testShrInt32() {
	var i int32 = -1
	println(i >> 1) // should be sign-extended
}

func main() {
	testShrUint32()
	testShrInt32()
}
