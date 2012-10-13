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
	"strings"
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
		for i, filename := range pkg.SFiles {
			pkg.SFiles[i] = path.Join(pkg.Dir, filename)
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

	args := []string{"-c", "-importpath", name, "-o", outfile}
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

	var output []byte
	var err error
	output, err = exec.Command(llgobin, "-print-triple").CombinedOutput()
	if err != nil {
		return err
	}
	triple := strings.TrimSpace(string(output))

	// Create the package directory.
	outdir := path.Join(runtime.GOROOT(), "pkg", "llgo", triple)
	err = os.MkdirAll(outdir, os.FileMode(0755))
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
