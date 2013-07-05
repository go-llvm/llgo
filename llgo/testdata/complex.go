package main

func main() {
	var f32 float32
	var f64 float64
	c64 := complex(f32, f32)
	println(c64 == c64)
	c128 := complex(f64, f64)
	println(c128 == c128)
}
