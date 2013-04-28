package main

type T struct {
	value int
}

type T1 struct {
	T
}

func (t T) abc() {
	println(t.value)
}

func (t *T) def() {
	println(t.value)
}

func (t *T) ghi(v int) {
	println(v)
}

func f5() {
	var t1 T1
	t1.T.value = 888
	defer t1.abc()
}

func f4() {
	var a T = T{999}
	var b *T = &a
	defer a.abc()
	defer a.def()
	defer a.ghi(123)
	defer b.abc()
	defer b.def()
	defer b.ghi(456)
	f5()
}

func f3() (a int) {
	defer func() { a *= 2 }()
	f4()
	return 123
}

func f2() {
	defer func() { println("f2.3") }()
	defer func(s string) { println(s) }("f2.2")
	println("f2.1")
	println(f3())
}

func f1() {
	defer func() { println("f1.2") }()
	defer func() { println("f1.1") }()
	f2()
}

func main() {
	f1()
}
