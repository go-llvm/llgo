// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	switch "abc" {
	case "def":
		println("def")
	case "abc":
		println("abc")
	}

	switch "abc" {
	case "def", "abc":
		println("def, abc")
	}
}
