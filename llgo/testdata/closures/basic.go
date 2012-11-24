package main

func cat(a, b string) func(string) string {
	return func(c string) string { return a + b + c }
}

func main() {
	f := cat("a", "b")
	println(f("c"))
}
