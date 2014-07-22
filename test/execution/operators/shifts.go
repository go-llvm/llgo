// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

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
