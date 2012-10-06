package main

func blah() []int {
	return make([]int, 1)
}

func main() {
	println(blah()[0])
}

