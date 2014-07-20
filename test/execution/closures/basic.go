// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func cat(a, b string) func(string) string {
	return func(c string) string { return a + b + c }
}

func main() {
	f := cat("a", "b")
	println(f("c"))
}
