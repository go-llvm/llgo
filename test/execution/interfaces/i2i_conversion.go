// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

type Numbered interface {
	Number() int
}

type Named interface {
	Name() string
}

type Beast struct{}

func (b *Beast) Number() int {
	return 666
}

func (b *Beast) Name() string {
	return "The Beast"
}

func main() {
	var b Beast
	var numbered Numbered = &b
	var named Named = numbered.(Named)
	println(numbered.Number())
	println(named.Name())
}
