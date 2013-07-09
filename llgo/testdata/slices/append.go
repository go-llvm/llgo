package main

func stringtobytes() {
	var b []byte
	b = append(b, "abc"...)
	b = append(b, "def"...)
	println(string(b))
}

func appendnothing() {
	var x []string
	println(append(x) == nil)
	x = append(x, "!")
	println(len(append(x)) == 1)
}

func main() {
	x := []int{}
	for i := 0; i < 100; i++ {
		x = append(x, i)
	}
	for i := 0; i < len(x); i++ {
		println(x[i])
	}
	y := []int{1, 2, 3}
	x = append(x, y...)
	for i := 0; i < len(x); i++ {
		println(x[i])
	}
	stringtobytes()
	appendnothing()
}
