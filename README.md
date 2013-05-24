[![Build Status](https://drone.io/github.com/axw/llgo/status.png)](https://drone.io/github.com/axw/llgo/latest)
# llgo

llgo is a compiler for [Go](http://golang.org), written in Go, and using the
[LLVM](http://llvm.org) compiler infrastructure.

llgo is a fledgling, and is being developed primarily as an educational
exercise. It is not presently useful for real work. Progress will be reported
at [http://blog.awilkins.id.au](http://blog.awilkins.id.au).

# Installation

The recommended way to install llgo is to use ```go get```. You'll need to set a
couple of environment variables first:

    export CGO_CFLAGS="`llvm-config --cflags`"
    export CGO_LDFLAGS="`llvm-config --ldflags` -Wl,-L`llvm-config --libdir` -lLLVM-`llvm-config --version`"
    go get github.com/axw/llgo/llgo

You must have LLVM 3.2 or better in your path. You can also use latest development
version of LLVM, and build it from [LLVM SVN repository](http://llvm.org/docs/GettingStarted.html#checkout),
just be sure to pass ```--enable-shared``` to LLVM ```configure``` during build.

Note: llgo requires Go 1.0.3 or Go tip before rev 2e2a1d8184e6 (last known good
rev is rev 6e9d872ffc66).

# Running

First compile the runtime support packages using cmd/llgo-dist command, just
run ```llgo-dist```.

Currently there is just a compiler which produces LLVM bitcode, and there is no
integration with the go command or cgo, etc. To compile a Go source file, simply
run ```llgo <file.go>```, which will emit LLVM bitcode to stdout. To produce
human-readable LLVM assembly code, supply an additional ```-dump``` command
line argument before ```<file.go>```.

# Testing

First make sure you have LLVM 3.2+ installed, then re-install gollvm package with
the tag llvmsvn.

	go clean -i github.com/axw/gollvm/llvm
	go install -tags llvmsvn github.com/axw/gollvm/llvm
	go test -i github.com/axw/llgo/llgo
	go test -v github.com/axw/llgo/llgo

