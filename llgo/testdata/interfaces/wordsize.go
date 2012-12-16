package main

type Stringer interface {
	String() string
}

type StringStringer string

func (s StringStringer) String() string {
	return "StringStringer(" + string(s) + ")"
}

func testLargerThanWord() {
	// string is larger than a word. Make sure it works
	// well as a method receiver when using interfaces.
	var s Stringer = StringStringer("abc")
	println(s.String())
}

func main() {
	testLargerThanWord()
}
