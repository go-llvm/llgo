package main

func testBytesConversion() {
	s := "abc"
	b := []byte(s)
	println("testBytesConversion:", s == string(b))
	b[0] = '!'
	println(s)
	s = string(b)
	b[0] = 'a'
	println(s)
}

func testBytesCopy() {
	s := "abc"
	b := make([]byte, len(s))
	copy(b, s)
	println("testBytesCopy:", string(b) == s)
}

func main() {
	testBytesConversion()
	testBytesCopy()
}
