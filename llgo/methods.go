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
)

// fixMethodDecls will walk through a file and associate functions with their
// receiver type.
func (self *compiler) fixMethodDecls(file *ast.File) {
    if file.Decls == nil {return}
    for _, decl := range file.Decls {
        if funcdecl, ok := decl.(*ast.FuncDecl); ok {
            if funcdecl.Recv != nil && funcdecl.Recv.List != nil {
                if len(funcdecl.Recv.List) != 1 {
                    panic("Expected exactly one receiver")
                }
                field := funcdecl.Recv.List[0]
                typ := self.GetType(field.Type)

                // Receivers may be a pointer type, or a "base" type.
                // TODO handle pointer base types (e.g. a declared type whose
                // base type is a pointer).
                typ = Deref(typ)

                // Insert a reference to the FuncDecl's AST object.
                typeinfo := self.typeinfo[typ]
                if typeinfo == nil {
                    typeinfo = &TypeInfo{}
                    self.typeinfo[typ] = typeinfo
                }
                if typeinfo.Methods == nil {
                    typeinfo.Methods = make(map[string]*ast.Object)
                }

                // The Obj field of the funcdecl wll be nil, so we'll have to
                // create a new one.
                funcdecl.Name.Obj = ast.NewObj(ast.Fun, funcdecl.Name.String())
                funcdecl.Name.Obj.Decl = funcdecl
                typeinfo.Methods[funcdecl.Name.String()] = funcdecl.Name.Obj
            }
        }
    }
}

// vim: set ft=go :

