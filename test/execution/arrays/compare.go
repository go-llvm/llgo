// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	a := [...]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	b := [...]int{10, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	c := [...]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	println(a == b)
	println(a == c)
	println(b == c)
}
