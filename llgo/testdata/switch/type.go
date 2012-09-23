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

type stringer interface {
	String() string
}

func printany(i interface{}) {
	switch v := i.(type) {
	case nil:
		print("nil", v)
	case stringer:
		print(v.String())
	case error:
		print(v.Error())
	case int:
		print(v)
	case string:
		print(v)
	}
}

func main() {
	test(int64(123))
	test("abc")
}

