// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	var a [10]int
	b := a[1:]
	println(len(a))
	println(len(b))
}
