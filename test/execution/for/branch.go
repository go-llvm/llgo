// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	for i := 0; true; i++ {
		println(i)
		if i == 2 {
			println(3)
			break
		}
		println(1)
		i++
		continue
		println("unreachable")
	}

	nums := [...]int{0, 1, 2, 3, 4, 5}
	for n := range nums {
		if n == 1 {
			continue
		}
		println(n)
		if n == 4 {
			{
				break
			}
			println("!")
		}
	}
}
