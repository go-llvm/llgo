// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

type BI interface {
	B()
}

type AI interface {
	A()
	BI
}

type S struct{}

func (s S) A() {
	println("A")
}

func (s S) B() {
	println("B")
}

func main() {
	var ai AI = S{}
	ai.A()
	ai.B()
}
