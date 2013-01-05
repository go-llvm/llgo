// Copyright 2013 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"go/ast"
)

type function struct {
	*LLVMValue
	params, results []*ast.Object
}

type functionStack []*function

func (s *functionStack) push(v *function) {
	*s = append(*s, v)
}

func (s *functionStack) pop() *function {
	f := (*s)[len(*s)-1]
	*s = (*s)[:len(*s)-1]
	return f
}

func (s *functionStack) top() *function {
	return (*s)[len(*s)-1]
}

func fieldListObjects(list *ast.FieldList) (objects []*ast.Object) {
	if list != nil {
		for _, field := range list.List {
			for _, ident := range field.Names {
				var obj *ast.Object
				if ident != nil {
					if ident.Name != "" && ident.Name != "_" {
						obj = ident.Obj
					}
				}
				objects = append(objects, obj)
			}
		}
	}
	return
}
