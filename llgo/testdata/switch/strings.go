package main

func main() {
	switch "abc" {
	case "def":
		println("def")
	case "abc":
		println("abc")
	}

	switch "abc" {
	case "def", "abc":
		println("def, abc")
	}
}
