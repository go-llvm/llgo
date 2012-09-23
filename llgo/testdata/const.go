package main

const (
    a = iota*2
    A = 1
    B
    C
    D = Z + iota
)

const (
    Z = iota
    Big = 1<<31 - 1
    Big2 = -2147483648
    Big3 = 2147483647
)

const (
	expbits32  uint = 8
	bias32          = -1<<(expbits32-1) + 1
)

func main() {
    println(a)
    println(B)
    println(A, A)
    println(A, B, C, D)
    println(Big)
    println(Big2)
    println(Big3)
    println(bias32)
}

