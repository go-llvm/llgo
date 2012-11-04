package main

type S struct {
	a, b int
}

func main() {
	s1 := S{1, 2}
	s2 := S{1, 3}
	println(s1 == s1)
	println(s1 == s2)
	println(s1 != s1)
	println(s1 != s2)
}

