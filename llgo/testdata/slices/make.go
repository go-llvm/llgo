package main

func main() {
	x := make([]int, 10)
	x[5] = 666
	for i, val := range x {
		println(i, val)
	}
}
