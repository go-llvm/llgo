package main

func main() {
    a := [...]int{1:1, 2:2, 4:4}
    for i := 0; i < len(a); i++ {
        println(a[i])
    }
}

