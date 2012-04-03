package main

func Blah() int {
    println("woobie")
    return 123
}

var X = Y + Blah() // == 579
var Y = 123 + Z // == 456

const (
    _ = 333*iota
    Z
)

func main() {
    println(X, Y)
}

