// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	a := "abc"
	b := "123"
	c := a + b
	println(len(a), len(b), len(c))
	println(c)
}
