package main

type Stringer interface {
	String() string
}

type X int
type Y int

type Z1 struct {
	X
}

type Z2 struct {
	Stringer
}

func (x X) String() string {
	return "X()"
}

func (y *Y) String() string {
	return "Y()"
}

func makeX() X {
	return X(0)
}

func main() {
	var z Stringer = X(0)
	println(z.String())

	z = new(Y)
	println(z.String())

	z = Z1{}
	println(z.String())

	z = Z2{new(Y)}
	println(z.String())

	println(makeX().String())
}
