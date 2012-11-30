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

package llgo

import (
	"fmt"
	"github.com/axw/llgo/types"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"path"
	"sync"
)

var (
	parseReflectOnce   sync.Once
	parseReflectResult *ast.Package
	parseReflectError  error
)

func parseFile(fset *token.FileSet, name string) (*ast.File, error) {
	return parser.ParseFile(fset, name, nil, parser.DeclarationErrors)
}

func parseFiles(fset *token.FileSet, filenames []string) (files map[string]*ast.File, err error) {
	files = make(map[string]*ast.File)
	for _, filename := range filenames {
		var file *ast.File
		file, err = parseFile(fset, filename)
		if err != nil {
			return
		} else if file != nil {
			if files[filename] != nil {
				err = fmt.Errorf("%q: duplicate file", filename)
				return
			}
			files[filename] = file
		}
	}
	return
}

// parseReflect parses the standard "reflect" package, builds and type-checks
// its AST. This is used to generate runtime type structures, using the
// existing types.Type -> llvm.Type mapping code.
func (c *compiler) parseReflect() (*ast.Package, error) {
	parseReflectOnce.Do(func() { c.doParseReflect() })
	return parseReflectResult, parseReflectError
}

func (c *compiler) doParseReflect() {
	buildpkg, err := build.Import("reflect", "", 0)
	if err != nil {
		parseReflectResult, parseReflectError = nil, err
		return
	}

	filenames := make([]string, len(buildpkg.GoFiles))
	for i, f := range buildpkg.GoFiles {
		filenames[i] = path.Join(buildpkg.Dir, f)
	}
	fset := token.NewFileSet()
	files, err := parseFiles(fset, filenames)
	if err != nil {
		parseReflectResult, parseReflectError = nil, err
		return
	}

	pkg, err := ast.NewPackage(fset, files, types.GcImport, types.Universe)
	if err != nil {
		parseReflectResult, parseReflectError = nil, err
		return
	}

	_, err = types.Check("reflect", c, fset, pkg)
	if err != nil {
		parseReflectResult, parseReflectError = nil, err
		return
	}

	parseReflectResult, parseReflectError = pkg, nil
	return
}

// vim: set ft=go:
