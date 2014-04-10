package main

func main() {
	var s []int
	println(s == nil)
	println(s != nil)
	println(nil == s)
	println(nil != s)
	s = make([]int, 0)
	println(s == nil)
	println(s != nil)
	println(nil == s)
	println(nil != s)
}
