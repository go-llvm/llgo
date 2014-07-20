// RUN: llgo -o %t %s
// RUN: %t > %t1 2>&1
// RUN: go run %s > %t2 2>&1
// RUN: diff -u %t1 %t2

package main

import "io"

type rdr struct{}

func (r rdr) Read(b []byte) (int, error) {
	return 0, nil
}

func F(i interface{}) {
	_ = i.(io.Reader)
}

func main() {
	var r rdr
	F(r)
	F(&r)
}
