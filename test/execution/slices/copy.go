// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	a := make([]int, 10)
	b := make([]int, 10)
	for i, _ := range b {
		b[i] = 1
	}
	println(copy(a[:5], b)) // expect 5
	println(a[5])           // expect 0
}
