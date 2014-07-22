// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	var f func()
	println(f == nil)
	println(f != nil)
	println(nil == f)
	println(nil != f)
	f = func() {}
	println(f == nil)
	println(f != nil)
	println(nil == f)
	println(nil != f)
}
