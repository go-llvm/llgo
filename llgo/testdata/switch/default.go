package main

func main() {
	switch true {
	default:
		println("default")
	}

	switch {
	default:
		println("default")
	case true:
		println("true")
	}
}
