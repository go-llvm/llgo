package main

type Stringer interface {
	String() string
}

type X int
type Y int

func (x X) String() string {
	return "X()"
}

func (y *Y) String() string {
	return "Y()"
}

func main() {
	var z Stringer
	z = X(0)
	println(z.String())
	//var y Y
	//z = &y
	//println(z.String())
}

