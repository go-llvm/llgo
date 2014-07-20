// RUN: llgo -o %t %s
// RUN: %t 2>&1 | sort > %t1
// RUN: go run %s 2>&1 | sort > %t2
// RUN: diff -u %t1 %t2

package main

func main() {
	defer println("done")
	m := make(map[int]int)
	m[0] = 3
	m[1] = 4
	m[2] = 5
	for k := range m {
		println(k)
	}
	for k, _ := range m {
		println(k)
	}
	for _, v := range m {
		println(v)
	}
	for k, v := range m {
		println(k, v)
	}

	// test deletion.
	i := 0
	for k, _ := range m {
		i++
		delete(m, (k+1)%3)
		delete(m, (k+2)%3)
	}
	println(i)
}
