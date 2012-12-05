package main

func f1(b bool) bool {
	return b
}

func main() {
	x := false
	y := x
	x = !y
	println(x || y)
}
