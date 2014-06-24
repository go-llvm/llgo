package main

func main() {
	ch := make(chan int, uint8(1))

	ch <- 1
	println(<-ch)

	ch <- 2
	x, ok := <-ch
	println(x)
	println(ok)
}
