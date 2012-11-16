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

// genMath generates the standard math package.
func genMath() error {
	log.Println("Generating math package")

	// XXX currently just copying the already-generated Go files from the
	// standard library. We'll need to either translate the assembly files
	// (manually or automatically), or perhaps use compiler-rt.

	gopkg, err := build.Import("math", "", 0)
	if err != nil {
		return err
	}

	const llgopkgpath = "github.com/axw/llgo/pkg/math"
	llgopkg, err := build.Import(llgopkgpath, "", build.FindOnly)
	if err != nil {
		return err
	}

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
		log.Printf("- %s", filename)
	}

	return nil
}
