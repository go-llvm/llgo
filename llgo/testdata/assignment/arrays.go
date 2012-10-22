package main

var a1 = [...]float32 {1.0, 2.0, 3.0}

func main() {
	var a2 [3]float32
	a2 = a1
	println(a2[0])
	println(a2[1])
	println(a2[2])

	// broken due to lack of promotion of
	// stack to heap.
	//println(a2[0], a2[1], a2[2])
}

