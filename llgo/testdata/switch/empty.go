package main

func f() int {
    println("f was called")
    return 123
}

func main() {
    switch f() {}
}

