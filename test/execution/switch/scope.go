// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	// case clauses have their own scope.
	switch {
	case true, false:
		x := 1
		println(x)
		fallthrough
	default:
		x := 2
		println(x)
	}
}
