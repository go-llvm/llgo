// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

import "unsafe"

var global string

var hi = 0xFF00
var lo = 0xFF00

// borrowed from math package to avoid dependency on standard library
func float32bits(f float32) uint32 { return *(*uint32)(unsafe.Pointer(&f)) }
func float64bits(f float64) uint64 { return *(*uint64)(unsafe.Pointer(&f)) }

func main() {
	println(hi & 0x1000)
	println(hi & 0x0100)
	println(hi & 0x0010)
	println(hi & 0x0001)
	println(lo & 0x1000)
	println(lo & 0x0100)
	println(lo & 0x0010)
	println(lo & 0x0001)
	println(hi | lo)
	println(hi ^ hi)
	println(hi ^ lo)
	println(lo ^ lo)
	println(hi ^ 0x1000)
	println(hi ^ 0x0100)
	println(hi ^ 0x0010)
	println(hi ^ 0x0001)
	println(lo ^ 0x1000)
	println(lo ^ 0x0100)
	println(lo ^ 0x0010)
	println(lo ^ 0x0001)
	println(-123 >> 1)
	println(-123 << 1)

	var f float64 = 123.456
	f--
	println(f)

	// context of '&' op is used to type the untyped lhs
	// operand of the shift expression.
	shift := uint(2)
	println(uint64(0xFFFFFFFF) & (1<<shift - 1))
	println((1<<shift - 1) & uint64(0xFFFFFFFF))

	// rhs' type is converted lhs'
	println(uint32(1) << uint64(2))
	{
		var _uint64 uint64
		var _uint uint
		x := _uint64 >> (63 - _uint)
		if x == 2<<_uint {
			println("!")
		}
	}

	// There was a bug related to compound expressions involving
	// multiple binary logical operators.
	var a, b, c int
	if a == 0 && (b != 0 || c != 0) {
		println("!")
	}

	var si int = -123
	var ui int = 123
	println(^si)
	println(^ui)
	println(ui &^ 3)

	// test case from math/modf.go
	var x uint64 = 0xFFFFFFFFFFFFFFFF
	var e uint = 40
	x &^= 1<<(64-12-e) - 1
	println(x)

	// compare global to non-global
	println(new(string) == &global)

	// negative zero
	var f32 float32
	var f64 float64
	var c64 complex64
	var c128 complex128
	f32 = -f32
	f64 = -f64
	c64 = -c64
	c128 = -c128
	println(float32bits(f32))
	println(float64bits(f64))
	println(float32bits(real(c64)), float32bits(imag(c64)))
	println(float64bits(real(c128)), float64bits(imag(c128)))
}
