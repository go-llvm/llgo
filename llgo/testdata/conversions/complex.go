package main

func main() {
	var c64 complex64
	var c128 complex128
	c128 = complex128(c64)
	c64 = complex64(c128)
}
