// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

func parseFile(fset *token.FileSet, filename string) (*ast.File, error) {
	mode := parser.DeclarationErrors | parser.ParseComments
	return parser.ParseFile(fset, filename, nil, mode)
}

func parseFiles(fset *token.FileSet, filenames []string) ([]*ast.File, error) {
	filenames = append([]string{}, filenames...)
	var files []*ast.File
	for i, filename := range filenames {
		if i > 0 && filenames[i-1] == filename {
			return nil, fmt.Errorf("%q: duplicate file", filename)
		} else {
			if file, err := parseFile(fset, filename); err != nil {
				return nil, fmt.Errorf("%q: %v", err)
			} else {
				files = append(files, file)
			}
		}
	}
	return files, nil
}
