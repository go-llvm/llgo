// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func p(i ...int) {
	println(len(i))
	for j := 0; j < len(i); j++ {
		println(i[j])
	}
}

func main() {
	p(123, 456, 789)
	p(123, 456, 789, 101112)
	p([]int{1, 2, 3}...)
}
