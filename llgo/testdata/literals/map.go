package main

func main() {
	type IntMap map[int]int
	m := IntMap{0: 1, 2: 3}
    println(m==nil)
    println(len(m))
	println(m[0], m[1], m[2])
}
