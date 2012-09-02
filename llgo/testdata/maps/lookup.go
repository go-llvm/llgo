package main

func main() {
	m := make(map[int]int)
	v, ok := m[8]
	println(v, ok)
	//m[8] = 1
	//v, ok = m[8]
	//println(v, ok)
}
