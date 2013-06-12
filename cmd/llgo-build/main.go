// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	llgobuild "github.com/axw/llgo/build"
	"go/build"
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
	buildctx      *build.Context
)

func init() {
	flag.StringVar(&clang, "clang", defaultclang, "The path to the clang compiler")
	flag.StringVar(&triple, "triple", defaulttriple, "The target triple")
	flag.StringVar(&output, "o", "", "Output file")
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

	err = buildPackages(flag.Args())
	if err != nil {
		log.Fatal(err)
	}
}
