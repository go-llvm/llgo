[![Build Status](https://drone.io/github.com/go-llvm/llgo/status.png)](https://drone.io/github.com/go-llvm/llgo/latest)
# llgo

llgo is a [Go](http://golang.org) frontend for [LLVM](http://llvm.org), written in Go.

llgo is under active development. It compiles and passes most of the standard library test suite and a substantial portion of the gc test suite, but there are some corner cases that are known not to be handled correctly yet. Nevertheless it can compile modestly substantial programs (including itself; it is self hosting on x86-64 Linux).

Progress will be reported on the [mailing list](https://groups.google.com/d/forum/llgo-dev).

# Installation

To install llgo, use make:

    go get github.com/go-llvm/llgo
    cd $GOPATH/src/github.com/go-llvm/llgo
    make install prefix=/path/to/prefix

You may need to build LLVM. See GoLLVM's README.md for more information.

llgo requires Go 1.3 or later.

# Running

We install two binaries to `$prefix/bin`: `llgo` and `llgo-go`.

`llgo` is the compiler binary. It has a command line interface that is intended to be compatible to a large extent with `gccgo`.

`llgo-go` is a command line wrapper for `go`. It works like the regular `go` command except that it uses llgo to build.
