// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

import "unsafe"

func main() {
	var i [2]int
	i[0] = 123
	i[1] = 456
	ptr := &i[0]
	println(*ptr)
	ptr_i := unsafe.Pointer(ptr)
	ptr_i = unsafe.Pointer(uintptr(ptr_i) + unsafe.Sizeof(i[0]))
	ptr = (*int)(ptr_i)
	println(*ptr)
}
