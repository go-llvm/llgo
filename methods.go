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
	"sort"
)

// fixMethodDecls will walk through a file and associate functions with their
// receiver type.
//
// TODO This should all move into type checking.
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
			var name *types.Name
			if ptr, isptr := typ.(*types.Pointer); isptr {
				name = ptr.Base.(*types.Name)
			} else {
				name = typ.(*types.Name)
			}

			method_obj := funcdecl.Name.Obj
			if name.Methods == nil {
				name.Methods = types.ObjList{method_obj}
			} else {
				methods := name.Methods
				methodname := funcdecl.Name.String()
				i := sort.Search(len(methods), func(i int) bool {
					return methods[i].Name >= methodname})
				if i < len(methods) && methods[i].Name == methodname {
					panic("Duplicate method")
				} else if i <= 0 {
					name.Methods = append(types.ObjList{method_obj},methods...)
				} else if i >= len(methods) {
					name.Methods = append(methods, method_obj)
				} else {
					right := methods[i:]
					methods = methods[:i]
					methods = append(methods, method_obj)
					name.Methods = append(methods, right...)
				}
			}
		}
	}
}

// vim: set ft=go :
