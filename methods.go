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
	"github.com/axw/llgo/types"
	"go/ast"
)

// fixMethodDecls will walk through a file and associate functions with their
// receiver type.
func (c *compiler) fixMethodDecls(file *ast.File) {
	if file.Decls == nil {
		return
	}
	for _, decl := range file.Decls {
		if funcdecl, ok := decl.(*ast.FuncDecl); ok && funcdecl.Recv != nil {
			field := funcdecl.Recv.List[0]
			typ := c.GetType(field.Type)

			// The Obj field of the funcdecl wll be nil, so we'll have to
			// create a new one.
			funcdecl.Name.Obj = ast.NewObj(ast.Fun, funcdecl.Name.String())
			funcdecl.Name.Obj.Decl = funcdecl

			// Record the FuncDecl's AST object in the type's methodset.
			if ptr, isptr := typ.(*types.Pointer); isptr {
				typ := ptr.Base
				if name, isname := typ.(*types.Name); isname {
					typ = name.Underlying
				}
				typeinfo := c.types.lookup(typ)
				typeinfo.ptrmethods[funcdecl.Name.String()] = funcdecl.Name.Obj
			} else {
				if name, isname := typ.(*types.Name); isname {
					typ = name.Underlying
				}
				typeinfo := c.types.lookup(typ)
				typeinfo.methods[funcdecl.Name.String()] = funcdecl.Name.Obj
			}
		}
	}
}

// vim: set ft=go :
