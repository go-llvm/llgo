package main

import (
    "os"
    "net/http"
    "bufio"
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

    _ = &reader{}
}

