package main

func main() {
	a := [...]int{1: 1, 2: 2, 4: 4}
	for i, val := range a {
		println(i, val, a[i])
	}
}
