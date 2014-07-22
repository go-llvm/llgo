// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func main() {
	var x *int = nil
	println(x)

	if x == nil {
		println("x is nil")
	}

	var y interface{}
	var z interface{} = y
	if y == nil {
		println("y is nil")
	} else {
		println("y is not nil")
	}

	if z == nil {
		println("z is nil")
	} else {
		println("z is not nil")
	}
}
