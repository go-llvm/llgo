[![Build Status](https://drone.io/github.com/go-llvm/llgo/status.png)](https://drone.io/github.com/go-llvm/llgo/latest)
# llgo

llgo is a [Go](http://golang.org) frontend for [LLVM](http://llvm.org), written in Go.

llgo is under active development. It compiles and passes most of the standard library test suite and a substantial portion of the gc test suite, but there are some corner cases that are known not to be handled correctly yet. Nevertheless it can compile modestly substantial programs (including itself; it is self hosting on x86-64 Linux).

Progress will be reported on the [mailing list](https://groups.google.com/d/forum/llgo-dev).

# Installation

llgo requires:
* Go 1.3 or later.
* [CMake](http://cmake.org/) 2.8.8 or later (to build LLVM).
* A [modern C++ toolchain](http://llvm.org/docs/GettingStarted.html#getting-a-modern-host-c-toolchain) (to build LLVM).

Note that Ubuntu Precise is one Linux distribution which does not package a sufficiently new CMake or C++ toolchain.

If you built a newer GCC following the linked instructions above, you will need to set the following environment variables before proceeding:

    export PATH=/path/to/gcc-inst/bin:$PATH
    export LD_LIBRARY_PATH=/path/to/gcc-inst/lib64:$LD_LIBRARY_PATH
    export CC=`which gcc`
    export CXX=`which g++`
    export LIBGO_CFLAGS=--gcc-toolchain=/path/to/gcc-inst

To build and install llgo:

    # Ensure $GOPATH is set.
    go get -d github.com/go-llvm/llgo/cmd/gllgo
    cd $GOPATH/src/github.com/go-llvm/llvm
    ./update_llvm.sh -DCMAKE_BUILD_TYPE=Release -DLLVM_TARGETS_TO_BUILD=host
    cd ../llgo
    make install prefix=/path/to/prefix j=N  # where N is the number of cores on your machine.

# Running

We install two binaries to `$prefix/bin`: `llgo` and `llgo-go`.

`llgo` is the compiler binary. It has a command line interface that is intended to be compatible to a large extent with `gccgo`.

`llgo-go` is a command line wrapper for `go`. It works like the regular `go` command except that it uses llgo to build.
