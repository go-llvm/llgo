/*
Copyright (c) 2011, 2012 Andrew Wilkins <axwalk@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

// Portions (from gotype):
//     Copyright 2011 The Go Authors. All rights reserved.
//     Use of this source code is governed by a BSD-style
//     license that can be found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo"
	"github.com/axw/llgo/types"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"strings"
)

var dump *bool = flag.Bool(
	"dump", false,
	"Dump the AST to stderr instead of generating bitcode")

var exitCode = 0

func report(err error) {
	scanner.PrintError(os.Stderr, err)
	exitCode = 2
}

func parseFile(fset *token.FileSet, filename string) *ast.File {
	// parse entire file
	mode := parser.DeclarationErrors
	//if *allErrors {
	//    mode |= parser.SpuriousErrors
	//}
	//if *printTrace {
	//    mode |= parser.Trace
	//}
	file, err := parser.ParseFile(fset, filename, nil, mode)
	if err != nil {
		report(err)
		return nil
	}
	return file
}

func parseFiles(fset *token.FileSet,
	filenames []string) (files map[string]*ast.File) {
	files = make(map[string]*ast.File)
	for _, filename := range filenames {
		if file := parseFile(fset, filename); file != nil {
			if files[filename] != nil {
				report(errors.New(fmt.Sprintf("%q: duplicate file", filename)))
				continue
			}
			files[filename] = file
		}
	}
	return
}

func isGoFilename(filename string) bool {
	// ignore non-Go files
	return !strings.HasPrefix(filename, ".") &&
		strings.HasSuffix(filename, ".go")
}

func processFiles(filenames []string) {
	i := 0
	for _, filename := range filenames {
		switch _, err := os.Stat(filename); {
		case err != nil:
			report(err)
		default:
			filenames[i] = filename
			i++
		}
	}
	fset := token.NewFileSet()
	processPackage(fset, parseFiles(fset, filenames[0:i]))
}

func processPackage(fset *token.FileSet, files map[string]*ast.File) {
	// make a package (resolve all identifiers)
	pkg, err := ast.NewPackage(fset, files, types.GcImporter, types.Universe)
	if err != nil {
		report(err)
		return
	}
	_, err = types.Check(fset, pkg)
	if err != nil {
		report(err)
		return
	}

	// Build LLVM module(s).
	module, err := llgo.Compile(fset, pkg)
	if err == nil {
		defer module.Dispose()
		if *dump {
			module.Dump()
		} else {
			err := llvm.WriteBitcodeToFile(module.Module, os.Stdout)
			if err != nil {
				fmt.Println(err)
			}
		}
	} else {
		//fmt.Printf("llg.Compile(%v) failed: %v", file.Name, err)
		report(err)
	}
}

func main() {
	flag.Parse()
	processFiles(flag.Args())
	os.Exit(exitCode)
}

// vim: set ft=go :

