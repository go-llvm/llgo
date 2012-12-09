// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"go/build"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
)

const llgoPkgPrefix = "github.com/axw/llgo/pkg/"

func getPackage(pkgpath string) (*build.Package, error) {
	var err error
	var pkg *build.Package

	pkg, err = build.Import(pkgpath, "", 0)
	if err != nil {
		return nil, err
	} else {
		for i, filename := range pkg.GoFiles {
			pkg.GoFiles[i] = path.Join(pkg.Dir, filename)
		}
		for i, filename := range pkg.CFiles {
			pkg.CFiles[i] = path.Join(pkg.Dir, filename)
		}

		// Look for .ll files, treat them the same as .s.
		// TODO look for build tags in the .ll file, check filename
		// for GOOS/GOARCH, etc.
		var llfiles []string
		llfiles, err = filepath.Glob(pkg.Dir + "/*.ll")
		for _, file := range llfiles {
			pkg.SFiles = append(pkg.SFiles, file)
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
		outfile := path.Join(outdir, dir, file+".a")
		err = buildPackage(pkg.name, pkg.path, outfile)
		if err != nil {
			return err
		}
	}

	return nil
}
