// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

type T1 *T1

func count(t T1) int {
	if t == nil {
		return 1
	}
	return 1 + count(*t)
}

func testSelfPointer() {
	var a T1
	var b T1
	var c T1 = &b
	*c = &a
	println(count(c))
	println(count(&c))
}

func main() {
	testSelfPointer()
}
