// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func test(r rune) {
	println("test(", r, ")")
	s := string(r)
	println(s)
	for i := 0; i < len(s); i++ {
		println(s[i])
	}
	for i, r := range s {
		println(i, r)
	}
}

type namedRune rune

func testslice(r1 []rune) {
	s := string(r1)
	println(s)
	r2 := []rune(s)
	r3 := []namedRune(s)
	println(len(r1), len(r2), len(r3))
	if len(r2) == len(r1) && len(r3) == len(r1) {
		for i := range r2 {
			println(r1[i] == r2[i])
			println(r1[i] == rune(r3[i]))
		}
	}
}

func main() {
	var runes = []rune{'.', '©', '€', '𐐀'}
	test(runes[0])
	test(runes[1])
	test(runes[2])
	test(runes[3])
	testslice(runes)
}
