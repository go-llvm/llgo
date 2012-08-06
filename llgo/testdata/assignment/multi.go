package main

func xyz() (int, int) {
    return 123, 456
}

func main() {
    a, b := xyz()
    println(a, b)
}

