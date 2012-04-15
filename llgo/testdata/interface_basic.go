package main

type any interface{}

func main() {
    var x int = 123
    var y any = x
    //println(y.(int))
}

