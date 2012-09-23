package main

func main() {
	var x interface{}
	x = int64(123)
	switch x := x.(type) {
	case int64:
		println("int64", x)
	default:
		println("default")
	}
}

