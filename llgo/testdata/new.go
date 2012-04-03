package main

func main() {
    x := new(int)
    println(*x)
    *x = 2
    println(*x)
    *x = *x * *x
    println(*x)
}

