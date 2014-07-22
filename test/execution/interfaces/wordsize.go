// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

type Stringer interface {
	String() string
}

type StringStringer string

func (s StringStringer) String() string {
	return "StringStringer(" + string(s) + ")"
}

func (s StringStringer) MethodWithArgs(a, b, c int) {
	println(s, a, b, c)
}

type I interface {
	MethodWithArgs(a, b, c int)
}

func testLargerThanWord() {
	// string is larger than a word. Make sure it works
	// well as a method receiver when using interfaces.
	var s Stringer = StringStringer("abc")
	println(s.String())

	// Test calling a method which takes parameters
	// beyond the receiver.
	s.(I).MethodWithArgs(1, 2, 3)
}

func main() {
	testLargerThanWord()
}
