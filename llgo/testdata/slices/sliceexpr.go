package main

func main() {
	x := []int{1, 2, 3, 4}
	for i, val := range x[1:3] {
		println(i, val)
	}
	println("")
	for i, val := range x[2:] {
		println(i, val)
	}
	println("")
	for i, val := range x[:2] {
		println(i, val)
	}
	println("")
	for i, val := range x[:] {
		println(i, val)
	}
}
