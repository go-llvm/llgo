package main

func printchars(s string) {
	var x int
	for i, c := range s {
		// test loop-carried dependence (x++), introducing a Phi node
		x++
		println(i, c, x)
	}

	// now test with plain old assignment
	var i int
	var c rune
	for i, c = range s {
		println(i, c)
		if i == len(s)-1 {
			// test multiple branches to loop header
			continue
		}
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
