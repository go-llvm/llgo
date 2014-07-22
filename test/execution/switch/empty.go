// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func f() int {
	println("f was called")
	return 123
}

func main() {
	switch f() {
	}
}
