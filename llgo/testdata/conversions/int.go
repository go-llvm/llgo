package main

func signed(i32 int32) {
	println(uint32(i32))
	println(int64(i32))
	println(uint64(i32))
}

func unsigned(u32 uint32) {
	println(int32(u32))
	println(int64(u32))
	println(uint64(u32))
}

func main() {
	signed(1<<31 - 1)
	signed(-1 << 31)
	signed(0)
	unsigned(1<<32 - 1)
	unsigned(0)
	unsigned(1)
}
