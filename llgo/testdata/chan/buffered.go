package main

func main() {
	c := make(chan int)
	c1 := make(chan int, 1)
	f := func() {
		n, ok := <-c
		if ok {
			c1 <- n * 10
		} else {
			c1 <- -1
		}
	}
	for i := 0; i < 10; i++ {
		go f()
		c <- i + 1
		println(<-c1)
	}
	go f()
	close(c)
	println(<-c1)
}
