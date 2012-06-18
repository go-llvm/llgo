package main

type A struct {}
func (a *A) test() {
    println("A.test")
}

type B struct {
    A
}
func (b B) test() {
    println("B.test")
}

func main() {
    var b B
    b.A.test()
    b.test()
}

