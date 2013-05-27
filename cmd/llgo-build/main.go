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
	pkgroot       string
	buildctx      *build.Context
)

func init() {
	flag.StringVar(&triple, "triple", defaulttriple, "The target triple")
}

func main() {
	flag.Parse()

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

    var packages []*build.Package
	for _, pkgpath := range flag.Args() {
		log.Println(pkgpath)
		pkg, err := buildPackage(pkgpath)
		if err != nil {
			log.Fatal(err)
		}
        packages = append(packages, pkg)
	}
}
