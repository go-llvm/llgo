package main

func main() {
	var x int
	px := &x
	*px = 123
	println(x)
}
