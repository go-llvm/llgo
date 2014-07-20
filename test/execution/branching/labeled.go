// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func labeledBreak() {
	var i int
L:
	for ; i < 10; i++ {
		switch {
		default:
			break L
		}
	}
	println(i)
}

func main() {
	labeledBreak()
}
