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
	"go/ast"
	"go/token"
)

// fixConstDecls will walk through a file and update ConstDecl's so that each
// ValueSpec has a valid Values Expr-list.
func (c *compiler) fixConstDecls(file *ast.File) {
	if file.Decls == nil {
		return
	}
	for _, decl := range file.Decls {
		if gendecl, ok := decl.(*ast.GenDecl); ok {
			if gendecl.Tok == token.CONST {
				var expr_valspec *ast.ValueSpec
				for _, spec := range gendecl.Specs {
					valspec, ok := spec.(*ast.ValueSpec)
					if !ok {
						panic("Expected *ValueSpec")
					}
					if valspec.Values != nil && len(valspec.Values) > 0 {
						expr_valspec = valspec
					} else {
						valspec.Type = expr_valspec.Type
						valspec.Values = expr_valspec.Values
					}
				}
			}
		}
	}
}

// vim: set ft=go :
