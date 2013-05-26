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
	// Make a copy, as we'll be modifying ReadDir/OpenFile.
	buildctx := *buildctx

	// Attempt to find an overlay package path,
	// which we'll use in ReadDir below.
	overlayentries := make(map[string]bool)
	overlaypkgpath := llgoPkgPrefix + pkgpath
	overlaypkg, err := buildctx.Import(overlaypkgpath, "", gobuild.FindOnly)
	if err != nil {
		overlaypkg = nil
	}

	// ReadDir is overridden to return a fake ".s"
	// file for each ".ll" file in the directory.
	buildctx.ReadDir = func(dir string) (fi []os.FileInfo, err error) {
		fi, err = ioutil.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		entries := make(map[string]os.FileInfo)
		for _, info := range fi {
			entries[info.Name()] = info
		}
		// Overlay all files in the overlay package dir.
		// If we find any .ll files, replace the suffix
		// with .s.
		if overlaypkg != nil {
			fi, err = ioutil.ReadDir(overlaypkg.Dir)
		}
		if err == nil {
			// Check for .ll files in the overlay dir if
			// we have one, else in the standard package dir.
			for _, info := range fi {
				name := info.Name()
				if strings.HasSuffix(name, ".ll") {
					name = name[:len(name)-3] + ".s"
					info = &renamedFileInfo{info, name}
				}
				overlayentries[name] = true
				entries[name] = info
			}
		}
		fi = make([]os.FileInfo, 0, len(entries))
		for _, info := range entries {
			fi = append(fi, info)
		}
		return fi, nil
	}

	// OpenFile is overridden to return the contents
	// of the ".ll" file found in ReadDir above. The
	// returned ReadCloser is wrapped to transform
	// LLVM IR comments to use "//", as expected by
	// go/build when looking for build tags.
	buildctx.OpenFile = func(path string) (io.ReadCloser, error) {
		base := filepath.Base(path)
		overlay := overlayentries[base]
		if overlay {
			if overlaypkg != nil {
				path = filepath.Join(overlaypkg.Dir, base)
			}
			if strings.HasSuffix(path, ".s") {
				path := path[:len(path)-2] + ".ll"
				var r io.ReadCloser
				var err error
				r, err = os.Open(path)
				if err == nil {
					r = build.NewLLVMIRReader(r)
				}
				return r, err
			}
		}
		return os.Open(path)
	}

	pkg, err := buildctx.Import(pkgpath, "", 0)
	if err != nil {
		return nil, err
	} else {
		if overlaypkg == nil {
			overlaypkg = pkg
		}
		for i, filename := range pkg.GoFiles {
			pkgdir := pkg.Dir
			if overlayentries[filename] {
				pkgdir = overlaypkg.Dir
			}
			pkg.GoFiles[i] = path.Join(pkgdir, filename)
		}
		for i, filename := range pkg.CFiles {
			pkgdir := pkg.Dir
			if overlayentries[filename] {
				pkgdir = overlaypkg.Dir
			}
			pkg.CFiles[i] = path.Join(pkgdir, filename)
		}
		for i, filename := range pkg.SFiles {
			pkgdir := pkg.Dir
			if overlayentries[filename] {
				pkgdir = overlaypkg.Dir
				filename = filename[:len(filename)-2] + ".ll"
			} else {
				err := fmt.Errorf("No matching .ll file for %q", filename)
				return nil, err
			}
			pkg.SFiles[i] = path.Join(pkgdir, filename)
		}
	}
	return pkg, nil
}

// TODO move to external tool? llgo-build?
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

	// Compile and link .c files in.
	llvmlink := filepath.Join(llvmbindir, "llvm-link")
	clang := "clang" // TODO make configurable
	for _, cfile := range pkg.CFiles {
		bcfile := cfile + ".bc"
		args = []string{"-c", "-target", triple, "-emit-llvm", "-o", bcfile, cfile}
		cmd := exec.Command(clang, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			os.Remove(bcfile)
			return err
		}
		cmd = exec.Command(llvmlink, "-o", outfile, outfile, bcfile)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		os.Remove(bcfile)
		if err != nil {
			return err
		}
	}

	// Link .ll files in.
	if len(pkg.SFiles) > 0 {
		args = []string{"-o", outfile, outfile}
		args = append(args, pkg.SFiles...)
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

	// TODO just use "go list std"
	runtimePackages := [...]string{
		"runtime",
		"syscall",
		"sync/atomic",
		"math",
		"sync",
		"time",
		"os",
		"io",
		"fmt",
		"strconv",
		"errors",
		"reflect",
		"unicode/utf8",
	}
	for _, pkg := range runtimePackages {
		name := pkg
		if pkg == "runtime" {
			pkg = llgoPkgPrefix + "runtime"
		}
		log.Printf("- %s", name)
		dir, file := path.Split(name)
		outfile := path.Join(outdir, dir, file+".bc")
		err = buildPackage(name, pkg, outfile)
		if err != nil {
			return err
		}
	}

	return nil
}
