package main

// An extFloat represents an extended floating-point number, with more
// precision than a float64. It does not try to save bits: the
// number represented by the structure is mant*(2^exp), with a negative
// sign if neg is true.
type extFloat struct {
	mant uint64
	exp  int
	neg  bool
}

var smallPowersOfTen = [...]extFloat{
	{1 << 63, -63, false},        // 1
	{0xa << 60, -60, false},      // 1e1
	{0x64 << 57, -57, false},     // 1e2
	{0x3e8 << 54, -54, false},    // 1e3
	{0x2710 << 50, -50, false},   // 1e4
	{0x186a0 << 47, -47, false},  // 1e5
	{0xf4240 << 44, -44, false},  // 1e6
	{0x989680 << 40, -40, false}, // 1e7
}

func main() {
	for i := range smallPowersOfTen {
		s := smallPowersOfTen[i]
		println(s.mant, s.exp, s.neg)
	}
}

