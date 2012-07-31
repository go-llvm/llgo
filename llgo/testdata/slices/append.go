package main

func main() {
	x := []int{1, 2}
	//x = append(x, 3)
	_ = append(x, 3)
	for i := 0; i < len(x); i++ {
		println(x[i])
	}
}

