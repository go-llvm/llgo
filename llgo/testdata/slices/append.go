package main

func main() {
	x := []int{}
	for i := 0; i < 100; i++ {
		x = append(x, i)
	}
	for i := 0; i < len(x); i++ {
		println(x[i])
	}
}

