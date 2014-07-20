// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	switch true {
	default:
		break
		println("default")
	}

	switch true {
	case true:
		println("true")
		fallthrough
	case false:
		println("false")
	}
}
