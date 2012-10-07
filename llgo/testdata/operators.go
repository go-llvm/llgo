package main

var hi = 0xFF00
var lo = 0xFF00

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

	// context of '&' op is used to type the untyped lhs
	// operand of the shift expression.
	shift := uint(2)
	println(uint64(0xFFFFFFFF) & (1<<shift - 1))

	// rhs' type is converted lhs'
	println(uint32(1) << uint64(2))

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
}

