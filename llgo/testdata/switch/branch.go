package main

func main() {
	switch true {
	default:
		break
		println("default")
	}

	switch true {
	case true:
		println("true")
		fallthrough
	case false:
		println("false")
	}
}
