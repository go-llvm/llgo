package main

func False() bool {
    println("False()")
    return false
}

func True() bool {
    println("True()")
    return true
}

func main() {
    println(False() || False())
    println(False() || True())
    println(True()  || False())
    println(True()  || True())
    println(False() && False())
    println(False() && True())
    println(True()  && False())
    println(True()  && True())
}

