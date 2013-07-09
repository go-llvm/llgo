package main

func printchars(s string) {
	for i, c := range s {
		println(i, c)
	}

	// now test with plain old assignment
	var i int
	var c rune
	for i, c = range s {
		println(i, c)
	}
}

func main() {
	// 1 bytes
	printchars(".")

	// 2 bytes
	printchars("©")

	// 3 bytes
	printchars("€")

	// 4 bytes
	printchars("𐐀")

	// mixed
	printchars("Sale price: €0.99")

	// TODO add test cases for invalid sequences
}
