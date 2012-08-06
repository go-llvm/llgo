package main

type any interface{}

func main() {
    var x1, x2 int = 1, 2
    var y any = x1
    var z any = x2
    if y != z {
        println("expected: y != z")
    } else {
        println("unexpected: y == z")
    }
	/*
	if y == x1 {
		println("expected: y == x1")
	} else {
		println("unexpected: y == x1")
	}
	*/
    //println(y.(int))
}

