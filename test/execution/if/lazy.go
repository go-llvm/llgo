// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func False() bool {
	println("False()")
	return false
}

func True() bool {
	println("True()")
	return true
}

func main() {
	println(False() || False())
	println(False() || True())
	println(True() || False())
	println(True() || True())
	println(False() && False())
	println(False() && True())
	println(True() && False())
	println(True() && True())
}
