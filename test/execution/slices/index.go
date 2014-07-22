// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func blah() []int {
	return make([]int, 1)
}

func main() {
	println(blah()[0])
}
