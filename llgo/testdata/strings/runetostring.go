package main

func test(r rune) {
	s := string(r)
	println(s)
	for i := 0; i < len(s); i++ {
		println(s[i])
	}
	for i, r := range s {
		println(i, r)
	}
}

func main() {
	test('.')
	test('©')
	test('€')
	test('𐐀')
}
