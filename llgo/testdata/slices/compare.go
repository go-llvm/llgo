package main

func main() {
	var s []int
	println(s == nil)
	println(s != nil)
	s = make([]int, 0)
	println(s == nil)
	println(s != nil)
}
