// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	s := "abcdef"
	println(s[:])
	println(s[1:])
	println(s[:3])
	println(s[1:4])
}
