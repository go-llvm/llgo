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
	"runtime"
	"sort"
	"strings"
)

var dumpast *bool = flag.Bool(
	"dumpast", false,
	"Dump the AST to stderr and exit")

var dump *bool = flag.Bool(
	"dump", false,
	"Dump the LLVM assembly to stderr and exit")

var trace *bool = flag.Bool(
	"trace", false,
	"Trace the compilation process")

var version *bool = flag.Bool(
	"version", false,
	"Display version information and exit")

var os_ *string = flag.String("os", runtime.GOOS, "Set the target OS")
var arch *string = flag.String("arch", runtime.GOARCH, "Set the target architecture")

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

func parseFiles(fset *token.FileSet, filenames []string) (files map[string]*ast.File) {
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

func compileFiles(filenames []string) (*llgo.Module, error) {
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
	if i == 0 {
		return nil, errors.New("No Go source files were specified")
	}
	fset := token.NewFileSet()
	return compilePackage(fset, parseFiles(fset, filenames[0:i]))
}

func compilePackage(fset *token.FileSet, files map[string]*ast.File) (*llgo.Module, error) {
	// make a package (resolve all identifiers)
	pkg, err := ast.NewPackage(fset, files, types.GcImporter, types.Universe)
	if err != nil {
		report(err)
		return nil, err
	}

	exprTypes, err := types.Check(fset, pkg)
	if err != nil {
		report(err)
		return nil, err
	}

	if *dumpast {
		ast.Fprint(os.Stderr, fset, pkg, nil)
		os.Exit(0)
	}

	compiler := llgo.NewCompiler()
	compiler.SetTraceEnabled(*trace)
	compiler.SetTargetArch(*arch)
	compiler.SetTargetOs(*os_)
	return compiler.Compile(fset, pkg, exprTypes)
}

func displayVersion() {
	fmt.Println("llgo version", llgo.LLGOVersion)
	fmt.Println()

	fmt.Println("  Available targets:")
	longestTargetName := 0
	targetDescriptions := make(map[string]string)
	targetNames := make([]string, 0)
	for target := llvm.FirstTarget(); target.C != nil; target = target.NextTarget() {
		targetName := target.Name()
		targetNames = append(targetNames, targetName)
		targetDescriptions[targetName] = target.Description()
		if len(targetName) > longestTargetName {
			longestTargetName = len(targetName)
		}
	}
	sort.Strings(targetNames)
	for _, targetName := range targetNames {
		var paddingLen int = longestTargetName - len(targetName)
		fmt.Printf("    %s %*s %s\n", targetName, paddingLen+1, "-",
			targetDescriptions[targetName])
	}
	fmt.Println()

	os.Exit(0)
}

func main() {
	llvm.InitializeAllTargets()
	llvm.InitializeAllTargetMCs()
	llvm.InitializeAllTargetInfos()
	flag.Parse()
	if *version {
		displayVersion()
	}

	module, err := compileFiles(flag.Args())
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
	os.Exit(exitCode)
}

// vim: set ft=go :
