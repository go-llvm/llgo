// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

type X struct{}
type Y X

func main() {
	var x X
	px := &x
	py := (*Y)(&x)
	py = (*Y)(px)
	_ = py
}
