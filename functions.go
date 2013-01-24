// Copyright 2013 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"go/ast"
	"go/types"
)

type function struct {
	*LLVMValue
	results []*types.Var
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

type methodset []*types.Func

func (m *methodset) Lookup(name string) *types.Func {
	for _, f := range *m {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func (m *methodset) Insert(f *types.Func) {
	*m = append(*m, f)
}

func (c *compiler) methods(n *types.NamedType) *methodset {
	if m, ok := c.methodsets[n]; ok {
		return m
	}

	// We're not privy to *Func objects for methods in
	// imported packages, so we synthesise them.
	pkg := c.objectdata[n.Obj].Package
	methods := new(methodset)
	c.methodsets[n] = methods
	for _, m := range n.Methods {
		f := &types.Func {
			Name: m.Name,
			Type: m.Type,
		}
		ident := ast.NewIdent(f.Name)
		c.objects[ident] = f
		c.objectdata[f] = &ObjectData{
			Ident:   ident,
			Package: pkg,
		}
		methods.Insert(f)
	}
	return methods
}
