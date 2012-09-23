package main

func test(i interface{}) {
	switch x := i.(type) {
	case int64:
		println("int64", x)
	// FIXME
	//case string:
	//	println("string", x)
	default:
		println("default")
	}
}

func main() {
	test(int64(123))
	test("abc")
}

