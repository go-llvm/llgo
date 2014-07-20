// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

func constIntToComplex() complex128 {
	return 0
}

func main() {
	var c64 complex64
	var c128 complex128
	c128 = complex128(c64)
	c64 = complex64(c128)
}
