[![Build Status](https://drone.io/github.com/axw/llgo/status.png)](https://drone.io/github.com/axw/llgo/latest)
# llgo

llgo is a [Go](http://golang.org) frontend for [LLVM](http://llvm.org), written in Go.

llgo is under active development, but is still considered experimental. It is not presently useful for real work. Progress will be reported at [http://blog.awilkins.id.au](http://blog.awilkins.id.au).

# Installation

To install llgo, use llgo-dist:

    go get github.com/axw/llgo/cmd/llgo-dist
    llgo-dist

You should have the latest version of LLVM in your $PATH (3.3 has been confirmed to be compatible). If LLVM is not in $PATH, llgo-dist also has a flag that can specified to point at the LLVM installation: `-llvm-config=<path/to/llvm-config>`.

llgo requires Go 1.2+.

# Running

llgo-dist builds two binaries: there's `llgo`, the compiler; and there's `llgo-build`, which is a poor man's `go build` for llgo.

The compiler is comparable with `6g`: it takes a set of Go source files as arguments, and produces an object file. The output is an LLVM bitcode module. There are several flags that alter the behaviour: `-triple=<triple>` specifies the target LLVM triple to compile for; `-dump` causes llgo to dump the module in its textual IR form instead of generating bitcode.

The `llgo-build` tool accepts either Go filenames, or package names, just like `go build`. If the package is a command, then `llgo-build` will compile it, link in its dependencies, and translate the LLVM bitcode to a native binary. If you want an untranslated module, specify the `-emit-llvm` flag.

`llgo-build` has some additional flags for testing: `-run` causes `llgo-build` to execute and dispose of the resultant binary. Passing `-test` causes `llgo-build` to generate a test program for the specified package, just like `go test -c`.

# Testing

First install llgo using `llgo-dist`, as described above. Then you can run the functional tests like so:

	go test -v github.com/axw/llgo/llgo

You can also run the compiler tests from gc's source tree ($GOROOT/test) by specifying the build tag `go_test`:

	go test -v -tags go_test github.com/axw/llgo/llgo -run StandardTests
