package main

func testShrUint32(v uint32) {
	for i := uint(0); i <= 32; i++ {
		println(v >> i)
		println(v << i)
	}
}

func testShrInt32(v int32) {
	for i := uint(0); i <= 32; i++ {
		println(v >> i)
		println(v << i)
	}
}

func main() {
	testShrUint32(0xFFFFFFFF)
	testShrUint32(0xEFFFFFFF)
	testShrInt32(-1)
	testShrInt32(1)
}
