package main

func testBasics() {
	var i [2]int
	j := &i
	i[0] = 123
	i[1] = 456
	println(i[0], i[1])
	println(j[0], j[1])
	i[0]++
	i[1]--
	println(i[0], i[1])
	println(j[0], j[1])
}

func testByteIndex() {
	var a [255]int
	for i := 0; i < len(a); i++ {
		a[i] = i
	}
	for i := byte(0); i < byte(len(a)); i++ {
		println(a[i])
	}
}

func main() {
	//testBasics()
	testByteIndex()
}
