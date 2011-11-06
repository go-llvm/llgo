# llgo

llgo is a compiler for [Go](http://golang.org), written in Go, and using the
[LLVM](http://llvm.org) compiler infrastructure.

llgo is a fledgling, and is being developed primarily as an educational
exercise. It is not presently useful for real work. Progress will be reported
at [http://blog.awilkins.id.au](http//blog.awilkins.id.au).

# Installation

The recommended way to install llgo is to use goinstall. You'll need to set a
couple of environment variables first:

    $ export CGO_CFLAGS=`llvm-config --cflags`
    $ export CGO_LDFLAGS="`llvm-config --ldflags` -Wl,-L`llvm-config --libdir` -lLLVM-`llvm-config --version`"
    $ goinstall github.com/axw/llgo

# Running

Currently there is just a compiler which produces LLVM bitcode, and there is no
integration with gomake/goinstall/cgo, etc. To compile a Go source file, simply
run ```llgo <file.go>```, which will emit LLVM bitcode to stdout.
    

