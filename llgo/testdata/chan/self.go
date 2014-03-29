package main

func main() {
	ch := make(chan int, 1)

	ch <- 1
	println(<-ch)

	ch <- 2
	x, ok := <-ch
	println(x)
	println(ok)
}
