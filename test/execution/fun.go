// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

// vim: set ft=go :

package main

func test() func() int {
	return blah
}

func blah() int {
	return 123
}

func sret() (int, bool, bool) {
	return 123, true, false
}

func main() {
	f := test()
	println(2 * f())
	a, b, c := sret()
	println(a, b, c)
}
