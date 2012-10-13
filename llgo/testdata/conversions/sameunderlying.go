package main

type X struct {}
type Y X

func main() {
	var x X
	px := &x
	py := (*Y)(&x)
	py = (*Y)(px)
	_ = py
}

