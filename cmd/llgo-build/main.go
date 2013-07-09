// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	llgobuild "github.com/axw/llgo/build"
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

var (
	llvmbindir    string
	llgobin       string
	triple        string
	defaulttriple string
	clang         string
	defaultclang  string
	pkgroot       string
	output        string
	printcommands bool
	emitllvm      bool
	buildctx      *build.Context
	workdir       string
)

func init() {
	flag.StringVar(&clang, "clang", defaultclang, "The path to the clang compiler")
	flag.StringVar(&triple, "triple", defaulttriple, "The target triple")
	flag.StringVar(&output, "o", "", "Output file")
	flag.BoolVar(&emitllvm, "emit-llvm", false, "Emit LLVM bitcode instead of a native binary")
	flag.BoolVar(&printcommands, "x", false, "Print the commands")
}

func main() {
	flag.Parse()

	if triple == "" {
		log.Fatal("No default triple set")
	}

	if clang == "" {
		clang = "clang"
	}

	var err error
	buildctx, err = llgobuild.Context(triple)
	if err != nil {
		log.Fatal(err)
	}

	// pkgroot = $GOPATH/pkg/llgo/<triple>
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = runtime.GOROOT()
	} else {
		gopath = filepath.SplitList(gopath)[0]
	}
	pkgroot = filepath.Join(gopath, "pkg", "llgo", triple)

	// Create a temporary work dir.
	workdir, err = ioutil.TempDir("", "llgo")
	if err != nil {
		log.Fatal(err)
	}
	err = buildPackages(flag.Args())
	os.RemoveAll(workdir)
	if err != nil {
		log.Fatal(err)
	}
}
