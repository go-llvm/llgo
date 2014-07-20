// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	x := []int{1, 2, 3, 4}
	for i, val := range x[1:3] {
		println(i, val)
	}
	println("")
	for i, val := range x[2:] {
		println(i, val)
	}
	println("")
	for i, val := range x[:2] {
		println(i, val)
	}
	println("")
	for i, val := range x[:] {
		println(i, val)
	}
}
