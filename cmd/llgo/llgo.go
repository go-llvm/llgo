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

package main

import (
    "fmt"
    "flag"
    "go/parser"
    "go/ast"
    "go/token"
    "go/types"
    "os"
    "github.com/axw/gollvm/llvm"
    "github.com/axw/llgo"
)

var dump *bool = flag.Bool(
                    "dump", false,
                    "Dump the AST to stderr instead of generating bitcode")

func main() {
    flag.Parse()
    fset := token.NewFileSet()

    filenames := flag.Args()
    packages, err := parser.ParseFiles(fset, filenames, 0)
    if err != nil {
        fmt.Printf("ParseFiles failed: %s\n", err.String())
        os.Exit(1)
    }

    // Create a new scope for each package.
    for _, pkg := range packages {
        pkg.Scope = ast.NewScope(types.Universe)
        obj := ast.NewObj(ast.Pkg, pkg.Name)
        obj.Data = pkg.Scope
    }

    // Type check and fill in the AST.
    for _, pkg := range packages {
        // TODO Imports? Or will 'Check' fill it in?
        types.Check(fset, pkg)
        //fmt.Println(pkg.Imports)
    }

    // Build LLVM module(s).
    for _, pkg := range packages {
        file := ast.MergePackageFiles(pkg, 0)
        file.Scope = ast.NewScope(pkg.Scope)

        module, err := llgo.Compile(fset, file)
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
            fmt.Printf("llg.Compile(%v) failed: %v", file.Name, err)
        }
    }
}

// vim: set ft=go :

