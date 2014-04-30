[![Build Status](https://drone.io/github.com/go-llvm/llgo/status.png)](https://drone.io/github.com/go-llvm/llgo/latest)
# llgo

llgo is a [Go](http://golang.org) frontend for [LLVM](http://llvm.org), written in Go.

llgo is under active development, but is still considered experimental. It is not presently useful for real work. Progress will be reported at [http://blog.awilkins.id.au](http://blog.awilkins.id.au).

# Installation

To install llgo, use make:

    go get github.com/go-llvm/llgo
    cd $GOPATH/src/github.com/go-llvm/llgo
    make install prefix=/path/to/prefix

You may need to build LLVM. See GoLLVM's README.md for more information.

llgo requires Go 1.3, or Go 1.2 with a [bug fix](https://codereview.appspot.com/96790047/) applied. See also README.patches for a list of additional patches to apply.

# Running

We install two binaries to `$prefix/bin`: `llgo` and `llgo-go`.

`llgo` is the compiler binary. It has a command line interface that is intended to be compatible to a large extent with `gccgo`.

`llgo-go` is a command line wrapper for `go`. It works like the regular `go` command except that it uses llgo to build.
