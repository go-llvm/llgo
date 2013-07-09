// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/build"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// goFilesPackage creates a package for building a collection of Go files.
//
// This function is based on the function of the same name in cmd/go.
func goFilesPackage(gofiles []string) (*build.Package, error) {
	for _, f := range gofiles {
		if !strings.HasSuffix(f, ".go") {
			return nil, fmt.Errorf("named files must be .go files")
		}
	}

	buildctx := *buildctx
	buildctx.UseAllFiles = true

	// Synthesize fake "directory" that only shows the named files,
	// to make it look like this is a standard package or
	// command directory.  So that local imports resolve
	// consistently, the files must all be in the same directory.
	var dirent []os.FileInfo
	var dir string
	for _, file := range gofiles {
		fi, err := os.Stat(file)
		if err != nil {
			return nil, err
		}
		if fi.IsDir() {
			return nil, fmt.Errorf("%s is a directory, should be a Go file", file)
		}
		dir1, _ := filepath.Split(file)
		if dir == "" {
			dir = dir1
		} else if dir != dir1 {
			fmt.Errorf("named files must all be in one directory; have %s and %s", dir, dir1)
		}
		dirent = append(dirent, fi)
	}
	buildctx.ReadDir = func(string) ([]os.FileInfo, error) { return dirent, nil }

	if !filepath.IsAbs(dir) {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(cwd, dir)
	}
	return buildctx.ImportDir(dir, 0)
}

func moveFile(src, dst string) error {
	if printcommands {
		log.Printf("mv %s %s\n", src, dst)
	}
	if os.Rename(src, dst) != nil {
		// rename may fail if the paths are on
		// different filesystems.
		fin, err := os.Open(src)
		if err != nil {
			return err
		}
		defer fin.Close()

		fout, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer fout.Close()

		info, err := fin.Stat()
		if err != nil {
			return err
		}
		if err = fout.Chmod(info.Mode()); err != nil {
			return err
		}
		if _, err = io.Copy(fout, fin); err != nil {
			return err
		}
		return os.Remove(src)
	}
	return nil
}
