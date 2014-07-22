// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	c := make(chan int)
	println(len(c), cap(c))
	c1 := make(chan int, 1)
	println(len(c1), cap(c1))
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
