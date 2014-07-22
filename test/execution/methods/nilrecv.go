// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

type T1 int

func (t *T1) t1() { println(t == nil) }

func constNilRecv() {
	(*T1)(nil).t1()
}

func nonConstNilRecv() {
	var v1 T1
	v1.t1()
	var v2 *T1
	v2.t1()
	v2 = &v1
	v2.t1()
}

func main() {
	constNilRecv()
	nonConstNilRecv()
}
