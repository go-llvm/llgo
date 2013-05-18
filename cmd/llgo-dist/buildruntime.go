// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/axw/llgo/build"
	gobuild "go/build"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const llgoPkgPrefix = "github.com/axw/llgo/pkg/"

type renamedFileInfo struct {
	os.FileInfo
	name string
}

func (r *renamedFileInfo) Name() string {
	return r.name
}

func getPackage(pkgpath string) (*gobuild.Package, error) {
	var ctx gobuild.Context = gobuild.Default
	ctx.GOARCH = GOARCH
	ctx.GOOS = GOOS
	ctx.BuildTags = append(ctx.BuildTags[:], "llgo")
	//ctx.Compiler = "llgo"

	// ReadDir is overridden to return a fake ".s"
	// file for each ".ll" file in the directory.
	ctx.ReadDir = func(dir string) (fi []os.FileInfo, err error) {
		fi, err = ioutil.ReadDir(dir)
		for i, info := range fi {
			name := info.Name()
			if strings.HasSuffix(name, ".ll") {
				name = name[:len(name)-3] + ".s"
				fi[i] = &renamedFileInfo{info, name}
			}
		}
		return
	}

	// OpenFile is overridden to return the contents
	// of the ".ll" file found in ReadDir above. The
	// returned ReadCloser is wrapped to transform
	// LLVM IR comments to use "//", as expected by
	// go/build when looking for build tags.
	ctx.OpenFile = func(path string) (io.ReadCloser, error) {
		if strings.HasSuffix(path, ".s") {
			origpath := path
			path := path[:len(path)-2] + ".ll"
			if _, err := os.Stat(path); err == nil {
				var r io.ReadCloser
				r, err = os.Open(path)
				if err == nil {
					r = build.NewLLVMIRReader(r)
				}
				return r, err
			} else {
				err = fmt.Errorf("No matching .ll file for %q", origpath)
				return nil, err
			}
		}
		return os.Open(path)
	}

	pkg, err := ctx.Import(pkgpath, "", 0)
	if err != nil {
		return nil, err
	} else {
		for i, filename := range pkg.GoFiles {
			pkg.GoFiles[i] = path.Join(pkg.Dir, filename)
		}
		for i, filename := range pkg.CFiles {
			pkg.CFiles[i] = path.Join(pkg.Dir, filename)
		}
		for i, filename := range pkg.SFiles {
			filename = filename[:len(filename)-2] + ".ll"
			pkg.SFiles[i] = path.Join(pkg.Dir, filename)
		}
	}
	return pkg, nil
}

func buildPackage(name, pkgpath, outfile string) error {
	dir, _ := path.Split(outfile)
	err := os.MkdirAll(dir, os.FileMode(0755))
	if err != nil {
		return err
	}

	pkg, err := getPackage(pkgpath)
	if err != nil {
		return err
	}

	args := []string{"-c", "-triple", triple, "-importpath", name, "-o", outfile}
	args = append(args, pkg.GoFiles...)
	cmd := exec.Command(llgobin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}

	// Link .ll files in.
	if len(pkg.SFiles) > 0 {
		args = []string{"-o", outfile, outfile}
		args = append(args, pkg.SFiles...)
		llvmlink := filepath.Join(llvmbindir, "llvm-link")
		cmd := exec.Command(llvmlink, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			return err
		}
	}

	return nil
}

func buildRuntime() error {
	log.Println("Building runtime")

	// Create the package directory.
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = runtime.GOROOT()
	} else {
		gopath = filepath.SplitList(gopath)[0]
	}
	outdir := path.Join(gopath, "pkg", "llgo", triple)
	err := os.MkdirAll(outdir, os.FileMode(0755))
	if err != nil {
		return err
	}

	type Package struct {
		name string
		path string
	}
	runtimePackages := [...]Package{
		{"runtime", llgoPkgPrefix + "runtime"},
		{"syscall", llgoPkgPrefix + "syscall"},
		{"sync/atomic", llgoPkgPrefix + "sync/atomic"},
		{"math", llgoPkgPrefix + "math"},
		{"sync", "sync"},
	}
	for _, pkg := range runtimePackages {
		log.Printf("- %s", pkg.name)
		dir, file := path.Split(pkg.name)
		outfile := path.Join(outdir, dir, file+".bc")
		err = buildPackage(pkg.name, pkg.path, outfile)
		if err != nil {
			return err
		}
	}

	return nil
}
