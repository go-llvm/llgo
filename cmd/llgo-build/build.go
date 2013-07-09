// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	llgobuild "github.com/axw/llgo/build"
	"go/build"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
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

func runCmd(cmd *exec.Cmd) error {
	if printcommands {
		s := fmt.Sprint(cmd.Args)
		log.Println(s[1 : len(s)-1])
	}
	return cmd.Run()
}

func getPackage(pkgpath string) (pkg *build.Package, err error) {
	// "runtime" is special: it's mostly written from scratch,
	// so we don't both with the overlay.
	if pkgpath == "runtime" {
		pkgpath = llgoPkgPrefix + "runtime"
		defer func() { pkg.ImportPath = "runtime" }()
	}

	// Make a copy, as we'll be modifying ReadDir/OpenFile.
	buildctx := *buildctx

	// Attempt to find an overlay package path,
	// which we'll use in ReadDir below.
	overlayentries := make(map[string]bool)
	overlaypkgpath := llgoPkgPrefix + pkgpath
	overlaypkg, err := buildctx.Import(overlaypkgpath, "", build.FindOnly)
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
					r = llgobuild.NewLLVMIRReader(r)
				}
				return r, err
			}
		}
		return os.Open(path)
	}

	pkg, err = buildctx.Import(pkgpath, "", 0)
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

func buildPackages(pkgpaths []string) error {
	if len(pkgpaths) > 0 && strings.HasSuffix(pkgpaths[0], ".go") {
		pkg, err := goFilesPackage(pkgpaths)
		if err != nil {
			return err
		}
		output := output
		if output == "" && pkg.IsCommand() {
			first := pkg.GoFiles[0]
			output = first[:len(first)-len(".go")]
		}
		for i, filename := range pkg.GoFiles {
			pkg.GoFiles[i] = filepath.Join(pkg.Dir, filename)
		}
		log.Printf("building %s\n", pkg.Name)
		return buildPackage(pkg, output)
	}

	for _, pkgpath := range pkgpaths {
		log.Printf("building %s\n", pkgpath)
		pkg, err := getPackage(pkgpath)
		if err != nil {
			return err
		}
		err = buildPackage(pkg, output)
		if err != nil {
			return err
		}
	}
	return nil
}

func buildPackage(pkg *build.Package, output string) error {
	args := []string{"-c", "-triple", triple}
	dir, file := path.Split(pkg.ImportPath)
	if pkg.IsCommand() {
		if output == "" {
			output = file
		}
	} else {
		dir = filepath.Join(pkgroot, dir)
		err := os.MkdirAll(dir, os.FileMode(0755))
		if err != nil {
			return err
		}
		if output == "" {
			output = path.Join(dir, file+".bc")
		}
		args = append(args, "-importpath", pkg.ImportPath)
	}

	_, file = path.Split(output)
	tempfile := path.Join(workdir, file+".bc")
	args = append(args, "-o", tempfile)
	args = append(args, pkg.GoFiles...)
	cmd := exec.Command("llgo", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := runCmd(cmd)
	if err != nil {
		return err
	}

	var cgoCFLAGS []string
	if len(pkg.CFiles) > 0 {
		cgoCFLAGS = strings.Fields(os.Getenv("CGO_CFLAGS"))
	}

	// Compile and link .c files in.
	llvmlink := filepath.Join(llvmbindir, "llvm-link")
	for _, cfile := range pkg.CFiles {
		bcfile := cfile + ".bc"
		args = []string{"-c", "-o", bcfile}
		if triple != "pnacl" {
			args = append(args, "-target", triple, "-emit-llvm")
		}
		args = append(args, cgoCFLAGS...)
		args = append(args, cfile)
		cmd := exec.Command(clang, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = runCmd(cmd)
		if err != nil {
			os.Remove(bcfile)
			return err
		}
		cmd = exec.Command(llvmlink, "-o", tempfile, tempfile, bcfile)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = runCmd(cmd)
		os.Remove(bcfile)
		if err != nil {
			return err
		}
	}

	// Link .ll files in.
	if len(pkg.SFiles) > 0 {
		args = []string{"-o", tempfile, tempfile}
		args = append(args, pkg.SFiles...)
		cmd := exec.Command(llvmlink, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = runCmd(cmd)
		if err != nil {
			return err
		}
	}

	// If it's a command, link in the dependencies.
	if pkg.IsCommand() {
		linkdeps(pkg, tempfile)
	}
	return moveFile(tempfile, output)
}
