// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	{
		var m map[int]int
		println(len(m)) // 0
		println(m[123]) // 0, despite map being nil
	}

	{
		m := make(map[int]int)
		m[123] = 456
		println(len(m)) // 1
		println(m[123])
		m[123] = 789
		println(len(m)) // 1
		println(m[123])
	}
}
