// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func xyz() (int, int) {
	return 123, 456
}

func abc() (int, int) {
	var a, b = xyz()
	return a, b
}

type S struct {
	a int
	b int
}

func main() {
	a, b := xyz()
	println(a, b)
	b, a = abc()
	println(a, b)

	// swap
	println(a, b)
	a, b = b, a
	println(a, b)

	var s S
	s.a, s.b = a, b
	println(s.a, s.b)
}
