// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package build

import (
	"bytes"
	"io"
)

// LLVMIRReadCloser wraps an io.ReadCloser, transforming the leading
// comments of the file such that they are preceded with "//" rather
// than standard LLVM IR comments ("; blah").
//
// This type is intended to be used in the OpenFile field of
// go/build's Context type.
type LLVMIRReadCloser struct {
	io.ReadCloser
	buf *bytes.Buffer
}

func NewLLVMIRReader(r io.ReadCloser) *LLVMIRReadCloser {
	return &LLVMIRReadCloser{r, nil}
}

var slashslash = []byte("//")
var semicolon = []byte(";")

func (r *LLVMIRReadCloser) Read(p []byte) (n int, err error) {
	if r.buf != nil {
		n, err = r.buf.Read(p)
		if err == io.EOF {
			if r.ReadCloser != nil {
				err = nil
			}
			r.buf = nil
		}
		return
	}

	n, err = r.ReadCloser.Read(p)
	if n > 0 {
		// To simplify the translation we'll simply
		// replace all instances of ";" in the returned
		// data.
		p_ := bytes.Replace(p, semicolon, slashslash, -1)
		if len(p_) > len(p) {
			r.buf = bytes.NewBuffer(p_)
			n, err = r.buf.Read(p)
			// err should be nil, since n < len(p)
		}
	}
	return
}
