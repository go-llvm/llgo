package main

func test(l, c int) {
	var s []int
	if l != -1 {
		if c == -1 {
			s = make([]int, l)
		} else {
			s = make([]int, l, c)
		}
	}
	println(len(s), cap(s))
}

func main() {
	test(-1, -1)
	test(0, -1)
	test(0, 0)
	test(1, -1)
	test(1, 1)
	test(1, 2)

	// make sure capacity is transferred to slice
	s := make([]int, 5, 10)
	s1 := s[1:3]
	println(len(s1), cap(s1))

	s2 := append(s1, 999)
	println(len(s2), cap(s2))
	println(s2[2])
	println(s[3])
}

