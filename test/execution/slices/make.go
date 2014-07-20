// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	x := make([]int, 10)
	x[5] = 666
	for i, val := range x {
		println(i, val)
	}
}
