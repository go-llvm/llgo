package main

import (
	"bufio"
	"net/http"
	"os"
	"unicode"
)

type E struct {
	e *E
}

type S struct {
	*E
	a, b int
}

type reader struct {
	*bufio.Reader
	fd   *os.File
	resp *http.Response
}

func main() {
	s := &S{nil, 1, 2}
	println(s.a, s.b)
	s = &S{a: 1, b: 2}
	println(s.a, s.b)

	_ = &reader{}

	// ensure keys resolve when imported
	r := unicode.Range32{
		Lo:     0,
		Stride: 2,
		Hi:     1,
	}

	println(r.Lo, r.Hi, r.Stride)
}
