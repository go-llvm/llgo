// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func f1() {
	c := make(chan int, 1)
	for i := 0; i < 3; i++ {
		select {
		case n, _ := <-c:
			println("received", n)
			c = nil
		case c <- 123:
			println("sent a value")
		default:
			println("default")
		}
	}
}

func main() {
	f1()
}
