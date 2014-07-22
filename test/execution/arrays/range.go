// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	a := [...]int{1: 1, 2: 2, 4: 4}
	for i, val := range a {
		println(i, val, a[i])
	}
	for i, val := range [...]int{10, 20, 30} {
		println(i, val)
	}
}
