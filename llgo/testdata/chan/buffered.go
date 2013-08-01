package main

func main() {
	c := make(chan int, 1)
	go func() {
		c <- 123
	}()
	n := <-c
	println(n)
}
