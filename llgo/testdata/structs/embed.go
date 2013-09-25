package main

type A struct{ aval int }

func (a *A) test() {
	println("A.test", a.aval)
}

func (a *A) testA() {
	println("A.testA")
}

func (a A) testA2() {
	println("A.testA2")
}

type B struct {
	A
	bval int
}

func (b B) test() {
	println("B.test", b.bval)
}

type C struct {
	*B
	cval int
}

func main() {
	var b B
	b.aval = 1
	b.bval = 2
	b.A.test()
	b.A.testA()
	b.A.testA2()
	b.test()
	b.testA()
	b.testA2()

	var c C
	c.B = &b
	c.cval = 3
	c.testA()
	//c.testA2()
}
