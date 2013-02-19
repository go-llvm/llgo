// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

// genSyscall generates the standard syscall package.
func genSyscall() error {
	log.Println("Generating syscall package")

	// XXX currently just copying the already-generated Go files from the
	// standard library. In the future, we'll just copy the handwritten
	// bits and generate the other stuff using a pure-Go package.

	gopkg, err := build.Import("syscall", "", 0)
	if err != nil {
		return err
	}

	const llgopkgpath = "github.com/axw/llgo/pkg/syscall"
	llgopkg, err := build.Import(llgopkgpath, "", build.FindOnly)
	if err != nil {
		return err
	}

	var filenames string
	for _, filename := range gopkg.GoFiles {
		srcfile := filepath.Join(gopkg.Dir, filename)
		data, err := ioutil.ReadFile(srcfile)
		if err != nil {
			return err
		}
		dstfile := filepath.Join(llgopkg.Dir, filename)
		err = ioutil.WriteFile(dstfile, data, os.FileMode(0644))
		if err != nil {
			return err
		}
		filenames += " " + filename
	}
	log.Printf("-%s", filenames)

	return nil
}
