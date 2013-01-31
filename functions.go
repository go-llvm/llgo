// Copyright 2013 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"github.com/axw/gollvm/llvm"
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

type methodset struct {
	ptr    []*types.Func
	nonptr []*types.Func
}

// lookup looks up a method by name, and whether it may or may
// not have a pointer receiver.
func (m *methodset) lookup(name string, ptr bool) *types.Func {
	funcs := m.nonptr
	if ptr {
		funcs = m.ptr
	}
	for _, f := range funcs {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func (c *compiler) methodfunc(m *types.Method) *types.Func {
	f := c.methodfuncs[m]
	// We're not privy to *Func objects for methods in
	// imported packages, so we must synthesise them.
	if f == nil {
		f = &types.Func{
			Name: m.Name,
			Type: m.Type,
		}
		ident := ast.NewIdent(f.Name)
		c.methodfuncs[m] = f
		c.objects[ident] = f
		c.objectdata[f] = &ObjectData{
			Ident:   ident,
			Package: m.Pkg,
		}
	}
	return f
}

func (c *compiler) methods(n *types.NamedType) *methodset {
	if m, ok := c.methodsets[n]; ok {
		return m
	}

	methods := new(methodset)
	c.methodsets[n] = methods
	for _, m := range n.Methods {
		f := c.methodfunc(m)
		if m.Type.Recv.Type == n {
			methods.nonptr = append(methods.nonptr, f)

			// Create ptr-receiver forwarding method.
			f := c.promoteMethod(m, &types.Pointer{n}, []int{-1})
			methods.ptr = append(methods.ptr, f)
		} else {
			methods.ptr = append(methods.ptr, f)
		}
	}

	// Traverse embedded types, build forwarding methods if
	// original type is in the main package (otherwise just
	// declaring them).
	var curr []selectorCandidate
	if typ, ok := derefType(underlyingType(n)).(*types.Struct); ok {
		curr = append(curr, selectorCandidate{nil, typ})
	}
	for len(curr) > 0 {
		var next []selectorCandidate
		for _, candidate := range curr {
			typ := candidate.Type
			var isptr bool
			if ptr, ok := typ.(*types.Pointer); ok {
				isptr = true
				typ = ptr.Base
			}

			if named, ok := typ.(*types.NamedType); ok {
				for _, m := range named.Methods {
					if methods.lookup(m.Name, true) != nil {
						continue
					}
					indices := candidate.Indices[:]
					if isptr || m.Type.Recv.Type == named {
						indices = append(indices, -1)
						if isptr && m.Type.Recv.Type == named {
							indices = append(indices, -1)
						}
						f := c.promoteMethod(m, n, indices)
						methods.nonptr = append(methods.nonptr, f)
						f = c.promoteMethod(m, &types.Pointer{Base: n}, indices)
						methods.ptr = append(methods.ptr, f)
					} else {
						// The method set of *S also includes
						// promoted methods with receiver *T.
						f := c.promoteMethod(m, &types.Pointer{Base: n}, indices)
						methods.ptr = append(methods.ptr, f)
					}
				}
				typ = named.Underlying
			}

			switch typ := typ.(type) {
			case *types.Interface:
				for i, m := range typ.Methods {
					if methods.lookup(m.Name, true) == nil {
						indices := candidate.Indices[:]
						indices = append(indices, -1) // always load
						f := c.promoteInterfaceMethod(typ, i, n, indices)
						methods.nonptr = append(methods.nonptr, f)
						f = c.promoteInterfaceMethod(typ, i, &types.Pointer{Base: n}, indices)
						methods.ptr = append(methods.ptr, f)
					}
				}

			case *types.Struct:
				indices := candidate.Indices[:]
				if isptr {
					indices = append(indices, -1)
				}
				for i, field := range typ.Fields {
					if field.IsAnonymous {
						indices := append(indices[:], i)
						ftype := field.Type
						candidate := selectorCandidate{indices, ftype}
						next = append(next, candidate)
					}
				}
			}
		}
		curr = next
	}

	return methods
}

// promoteInterfaceMethod promotes an interface method to a type
// which has embedded the interface.
func (c *compiler) promoteInterfaceMethod(iface *types.Interface, methodIndex int, recv types.Type, indices []int) *types.Func {
	m := iface.Methods[methodIndex]
	sig := *m.Type
	sig.Recv = &types.Var{Type: recv}
	f := &types.Func{Name: m.Name, Type: &sig}
	ident := ast.NewIdent(f.Name)

	var isptr bool
	if ptr, ok := recv.(*types.Pointer); ok {
		isptr = true
		recv = ptr.Base
	}

	pkg := c.objectdata[recv.(*types.NamedType).Obj].Package
	c.objects[ident] = f
	c.objectdata[f] = &ObjectData{Ident: ident, Package: pkg}

	if pkg == c.pkg {
		defer c.builder.SetInsertPointAtEnd(c.builder.GetInsertBlock())
		llvmfn := c.Resolve(ident).LLVMValue()
		llvmfn = c.builder.CreateExtractValue(llvmfn, 0, "")
		entry := llvm.AddBasicBlock(llvmfn, "entry")
		c.builder.SetInsertPointAtEnd(entry)

		args := llvmfn.Params()
		ifaceval := args[0]
		if !isptr {
			ptr := c.builder.CreateAlloca(ifaceval.Type(), "")
			c.builder.CreateStore(ifaceval, ptr)
			ifaceval = ptr
		}
		for _, i := range indices {
			if i == -1 {
				ifaceval = c.builder.CreateLoad(ifaceval, "")
			} else {
				ifaceval = c.builder.CreateStructGEP(ifaceval, i, "")
			}
		}

		recvarg := c.builder.CreateExtractValue(ifaceval, 0, "")
		ifn := c.builder.CreateExtractValue(ifaceval, methodIndex+2, "")

		// Add the receiver argument type.
		fntyp := ifn.Type().ElementType()
		returnType := fntyp.ReturnType()
		paramTypes := fntyp.ParamTypes()
		paramTypes = append([]llvm.Type{recvarg.Type()}, paramTypes...)
		vararg := fntyp.IsFunctionVarArg()
		fntyp = llvm.FunctionType(returnType, paramTypes, vararg)
		fnptrtyp := llvm.PointerType(fntyp, 0)
		ifn = c.builder.CreateBitCast(ifn, fnptrtyp, "")

		args[0] = recvarg
		result := c.builder.CreateCall(ifn, args, "")
		if len(m.Type.Results) == 0 {
			c.builder.CreateRetVoid()
		} else {
			c.builder.CreateRet(result)
		}
	}

	return f
}

// promoteMethod promotes a named type's method to another type
// which has embedded the named type.
func (c *compiler) promoteMethod(m *types.Method, recv types.Type, indices []int) *types.Func {
	sig := *m.Type
	sig.Recv = &types.Var{Type: recv}
	f := &types.Func{Name: m.Name, Type: &sig}
	ident := ast.NewIdent(f.Name)

	var isptr bool
	if ptr, ok := recv.(*types.Pointer); ok {
		isptr = true
		recv = ptr.Base
	}

	pkg := c.objectdata[recv.(*types.NamedType).Obj].Package
	c.objects[ident] = f
	c.objectdata[f] = &ObjectData{Ident: ident, Package: pkg}

	if pkg == c.pkg {
		defer c.builder.SetInsertPointAtEnd(c.builder.GetInsertBlock())
		llvmfn := c.Resolve(ident).LLVMValue()
		llvmfn = c.builder.CreateExtractValue(llvmfn, 0, "")
		entry := llvm.AddBasicBlock(llvmfn, "entry")
		c.builder.SetInsertPointAtEnd(entry)

		methodfunc := c.methodfunc(m)
		realfn := c.Resolve(c.objectdata[methodfunc].Ident).LLVMValue()
		realfn = c.builder.CreateExtractValue(realfn, 0, "")

		args := llvmfn.Params()
		recvarg := args[0]
		if !isptr {
			ptr := c.builder.CreateAlloca(recvarg.Type(), "")
			c.builder.CreateStore(recvarg, ptr)
			recvarg = ptr
		}
		for _, i := range indices {
			if i == -1 {
				recvarg = c.builder.CreateLoad(recvarg, "")
			} else {
				recvarg = c.builder.CreateStructGEP(recvarg, i, "")
			}
		}

		args[0] = recvarg
		result := c.builder.CreateCall(realfn, args, "")
		if len(m.Type.Results) == 0 {
			c.builder.CreateRetVoid()
		} else {
			c.builder.CreateRet(result)
		}
	}
	return f
}
