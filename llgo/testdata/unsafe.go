package main

import "unsafe"

func main() {
    var i [2]int
    i[0] = 123
    i[1] = 456
    ptr := &i[0]
    println(*ptr)
    ptr_i := unsafe.Pointer(ptr)
    ptr_i = unsafe.Pointer(uintptr(ptr_i) + 4)
    ptr = (*int)(ptr_i)
    println(*ptr)
}

