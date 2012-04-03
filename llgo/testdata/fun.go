// vim: set ft=go :

package main

func test() (func() int) {
    return blah
}

func blah() int {
    return 123
}

func main() {
    f := test()
    println(2*f())
}

