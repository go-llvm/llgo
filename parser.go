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
	files := make([]*ast.File, len(filenames))
	for i, filename := range filenames {
		file, err := parseFile(fset, filename)
		if err != nil {
			return nil, fmt.Errorf("%q: %v", filename, err)
		}
		files[i] = file
	}
	return files, nil
}
