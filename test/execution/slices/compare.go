// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	var s []int
	println(s == nil)
	println(s != nil)
	println(nil == s)
	println(nil != s)
	s = make([]int, 0)
	println(s == nil)
	println(s != nil)
	println(nil == s)
	println(nil != s)
}
