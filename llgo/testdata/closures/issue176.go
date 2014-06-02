package main

func main() {
	a := false
	f := func() {
		make(chan *bool, 1) <- &a
	}
	f()
	println(a)
}
