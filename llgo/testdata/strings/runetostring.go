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

func testslice(r1 []rune) {
	s := string(r1)
	println(s)
	r2 := []rune(s)
	println(len(r1), len(r2))
	if len(r2) == len(r1) {
		for i := range r2 {
			println(r1[i] == r2[i])
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
