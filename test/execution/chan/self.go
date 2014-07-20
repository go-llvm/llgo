// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

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
