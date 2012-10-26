package main

type X struct{}
func (x *X) F1() { println("(*X).F1") }
func (x *X) F2() { println("(*X).F2") }

type I interface {
	F1()
	F2()
}

func main() {
	var x interface{}
	x = int32(123456)

	if i, ok := x.(I); ok {
		i.F1()
		_ = i
	} else {
		println("!")
	}
	x = &X{}
	if i, ok := x.(I); ok {
		i.F1()
		_ = i
	} else {
		println("!")
	}
}
