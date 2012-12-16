package main

type Numbered interface {
    Number() int
}

type Named interface {
    Name() string
}

type Beast struct {}

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

