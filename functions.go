// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"fmt"
	"github.com/greggoryhz/gollvm/llvm"
	"go/ast"
	"go/token"
	"sort"
)

type function struct {
	*LLVMValue
	results *types.Tuple

	unwindblock llvm.BasicBlock
	deferblock  llvm.BasicBlock
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
	if len(*s) > 0 {
		return (*s)[len(*s)-1]
	}
	return nil
}

type methodset struct {
	ptr    []types.Object
	nonptr []types.Object
}

// lookup looks up a method by name, and whether it may or may
// not have a pointer receiver.
func (m *methodset) lookup(name string, ptr bool) types.Object {
	funcs := m.nonptr
	if ptr {
		funcs = m.ptr
	}
	for _, f := range funcs {
		if f.Name() == name {
			return f
		}
	}
	return nil
}

func (c *compiler) methodfunc(m *types.Func) *types.Func {
	// We're not privy to *Func objects for methods in
	// imported packages, so we must synthesise them.
	data := c.objectdata[m]
	if data == nil {
		ident := ast.NewIdent(m.Name())
		data = &ObjectData{Ident: ident, Package: m.Pkg()}
		c.objects[ident] = m
		c.objectdata[m] = data
	}
	return m
}

// methods constructs and returns the method set for
// a named type or unnamed struct type.
func (c *compiler) methods(t types.Type) *methodset {
	if m, ok := c.methodsets[t]; ok {
		return m
	}

	methods := new(methodset)
	c.methodsets[t] = methods

	switch t := t.(type) {
	case *types.Named:
		for i := 0; i < t.NumMethods(); i++ {
			f := c.methodfunc(t.Method(i))
			if f.Type().(*types.Signature).Recv().Type() == t {
				methods.nonptr = append(methods.nonptr, f)
				f := c.promoteMethod(f, types.NewPointer(t), []int{-1})
				methods.ptr = append(methods.ptr, f)
			} else {
				methods.ptr = append(methods.ptr, f)
			}
		}
	case *types.Struct:
		// No-op, handled in loop below.
	default:
		panic(fmt.Errorf("non Named/Struct: %#v", t))
	}

	// Traverse embedded types, build forwarding methods if
	// original type is in the main package (otherwise just
	// declaring them).
	var curr []selectorCandidate
	if typ, ok := t.Underlying().Deref().(*types.Struct); ok {
		curr = append(curr, selectorCandidate{nil, typ})
	}
	for len(curr) > 0 {
		var next []selectorCandidate
		for _, candidate := range curr {
			typ := candidate.Type
			var isptr bool
			if ptr, ok := typ.(*types.Pointer); ok {
				isptr = true
				typ = ptr.Elem()
			}

			if named, ok := typ.(*types.Named); ok {
				for i := 0; i < named.NumMethods(); i++ {
					m := c.methodfunc(named.Method(i))
					if methods.lookup(m.Name(), true) != nil {
						continue
					}
					sig := m.Type().(*types.Signature)
					indices := candidate.Indices[:]
					if isptr || sig.Recv().Type() == named {
						indices = append(indices, -1)
						if isptr && sig.Recv().Type() == named {
							indices = append(indices, -1)
						}
						f := c.promoteMethod(m, t, indices)
						methods.nonptr = append(methods.nonptr, f)
						f = c.promoteMethod(m, types.NewPointer(t), indices)
						methods.ptr = append(methods.ptr, f)
					} else {
						// The method set of *S also includes
						// promoted methods with receiver *T.
						f := c.promoteMethod(m, types.NewPointer(t), indices)
						methods.ptr = append(methods.ptr, f)
					}
				}
				typ = named.Underlying()
			}

			switch typ := typ.(type) {
			case *types.Interface:
				for i, m := range sortedMethods(typ) {
					if methods.lookup(m.Name(), true) == nil {
						indices := candidate.Indices[:]
						indices = append(indices, -1) // always load
						f := c.promoteInterfaceMethod(typ, m, i, t, indices)
						methods.nonptr = append(methods.nonptr, f)
						f = c.promoteInterfaceMethod(typ, m, i, types.NewPointer(t), indices)
						methods.ptr = append(methods.ptr, f)
					}
				}

			case *types.Struct:
				indices := candidate.Indices[:]
				if isptr {
					indices = append(indices, -1)
				}
				for i := 0; i < typ.NumFields(); i++ {
					field := typ.Field(i)
					if field.Anonymous() {
						indices := append(indices[:], i)
						ftype := field.Type()
						candidate := selectorCandidate{indices, ftype}
						next = append(next, candidate)
					}
				}
			}
		}
		curr = next
	}

	sort.Sort(objectsByName(methods.ptr))
	sort.Sort(objectsByName(methods.nonptr))
	return methods
}

type synthFunc struct {
	// Func is the original funcion object
	// from which this object was derived.
	*types.Func
	pkg *types.Package
	typ types.Type
}

func (f *synthFunc) Pkg() *types.Package {
	return f.pkg
}

func (f *synthFunc) Outer() *types.Scope {
	return nil
}

func (f *synthFunc) Type() types.Type {
	return f.typ
}

func (*synthFunc) Pos() token.Pos {
	return token.NoPos
}

// promoteInterfaceMethod promotes an interface method to a type
// which has embedded the interface.
//
// TODO consolidate this and promoteMethod.
func (c *compiler) promoteInterfaceMethod(iface *types.Interface, m *types.Func, methodIndex int, recv types.Type, indices []int) types.Object {
	var pkg *types.Package
	if recv, ok := recv.(*types.Named); ok {
		pkg = c.objectdata[recv.Obj()].Package
	}
	recvvar := types.NewVar(token.NoPos, pkg, "", recv)
	sig := m.Type().(*types.Signature)
	sig = types.NewSignature(recvvar, sig.Params(), sig.Results(), sig.IsVariadic())
	f := &synthFunc{Func: m, pkg: pkg, typ: sig}
	ident := ast.NewIdent(f.Name())

	var isptr bool
	if ptr, ok := recv.(*types.Pointer); ok {
		isptr = true
		recv = ptr.Elem()
	}

	c.objects[ident] = f
	c.objectdata[f] = &ObjectData{Ident: ident, Package: pkg}

	if pkg == nil || pkg == c.pkg {
		if currblock := c.builder.GetInsertBlock(); !currblock.IsNil() {
			defer c.builder.SetInsertPointAtEnd(currblock)
		}
		llvmfn := c.Resolve(ident).LLVMValue()
		llvmfn = c.builder.CreateExtractValue(llvmfn, 0, "")
		llvmfn.SetLinkage(llvm.LinkOnceODRLinkage)
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
		if sig.Results().Len() == 0 {
			c.builder.CreateRetVoid()
		} else {
			c.builder.CreateRet(result)
		}
	}

	return f
}

// promoteMethod promotes a named type's method to another type
// which has embedded the named type.
func (c *compiler) promoteMethod(m *types.Func, recv types.Type, indices []int) types.Object {
	var pkg *types.Package
	if recv, ok := recv.(*types.Named); ok {
		pkg = c.objectdata[recv.Obj()].Package
	}
	recvvar := types.NewVar(token.NoPos, pkg, "", recv)
	sig := m.Type().(*types.Signature)
	sig = types.NewSignature(recvvar, sig.Params(), sig.Results(), sig.IsVariadic())
	f := &synthFunc{Func: m, pkg: pkg, typ: sig}
	ident := ast.NewIdent(f.Name())

	var isptr bool
	if ptr, ok := recv.(*types.Pointer); ok {
		isptr = true
		recv = ptr.Elem()
	}

	c.objects[ident] = f
	c.objectdata[f] = &ObjectData{Ident: ident, Package: pkg}

	if pkg == nil || pkg == c.pkg {
		if currblock := c.builder.GetInsertBlock(); !currblock.IsNil() {
			defer c.builder.SetInsertPointAtEnd(currblock)
		}
		llvmfn := c.Resolve(ident).LLVMValue()
		llvmfn = c.builder.CreateExtractValue(llvmfn, 0, "")
		llvmfn.SetLinkage(llvm.LinkOnceODRLinkage)
		entry := llvm.AddBasicBlock(llvmfn, "entry")
		c.builder.SetInsertPointAtEnd(entry)

		realfn := c.Resolve(c.objectdata[m].Ident).LLVMValue()
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
		if sig.Results().Len() == 0 {
			c.builder.CreateRetVoid()
		} else {
			c.builder.CreateRet(result)
		}
	}
	return f
}

// indirectFunction creates an indirect function from a
// given function, suitable for use with "defer" and "go".
func (c *compiler) indirectFunction(fn *LLVMValue, args []Value, dotdotdot bool) *LLVMValue {
	nilarytyp := &types.Signature{}
	if len(args) == 0 {
		val := fn.LLVMValue()
		ptr := c.builder.CreateExtractValue(val, 0, "")
		ctx := c.builder.CreateExtractValue(val, 1, "")
		fnval := llvm.Undef(c.types.ToLLVM(nilarytyp))
		ptr = c.builder.CreateBitCast(ptr, fnval.Type().StructElementTypes()[0], "")
		ctx = c.builder.CreateBitCast(ctx, fnval.Type().StructElementTypes()[1], "")
		fnval = c.builder.CreateInsertValue(fnval, ptr, 0, "")
		fnval = c.builder.CreateInsertValue(fnval, ctx, 1, "")
		fn = c.NewValue(fnval, nilarytyp)
		return fn
	}

	// TODO check if function pointer is global. I suppose
	// the same can be done with the context ptr...
	fnval := fn.LLVMValue()
	fnptr := c.builder.CreateExtractValue(fnval, 0, "")
	ctx := c.builder.CreateExtractValue(fnval, 1, "")
	nctx := 1 // fnptr
	if !ctx.IsNull() {
		nctx++ // fnctx
	}

	i8ptr := llvm.PointerType(llvm.Int8Type(), 0)
	llvmargs := make([]llvm.Value, len(args)+nctx)
	llvmargtypes := make([]llvm.Type, len(args)+nctx)
	for i, arg := range args {
		llvmargs[i+nctx] = arg.LLVMValue()
		llvmargtypes[i+nctx] = llvmargs[i+nctx].Type()
	}
	llvmargtypes[0] = fnptr.Type()
	llvmargs[0] = fnptr
	if nctx > 1 {
		llvmargtypes[1] = ctx.Type()
		llvmargs[1] = ctx
	}

	structtyp := llvm.StructType(llvmargtypes, false)
	argstruct := c.createTypeMalloc(structtyp)
	for i, llvmarg := range llvmargs {
		argptr := c.builder.CreateGEP(argstruct, []llvm.Value{
			llvm.ConstInt(llvm.Int32Type(), 0, false),
			llvm.ConstInt(llvm.Int32Type(), uint64(i), false)}, "")
		c.builder.CreateStore(llvmarg, argptr)
	}

	// Create a function that will take a pointer to a structure of the type
	// defined above, or no parameters if there are none to pass.
	fntype := llvm.FunctionType(llvm.VoidType(), []llvm.Type{argstruct.Type()}, false)
	indirectfn := llvm.AddFunction(c.module.Module, "", fntype)
	i8argstruct := c.builder.CreateBitCast(argstruct, i8ptr, "")
	currblock := c.builder.GetInsertBlock()
	c.builder.SetInsertPointAtEnd(llvm.AddBasicBlock(indirectfn, "entry"))
	argstruct = indirectfn.Param(0)
	for i := range llvmargs[nctx:] {
		argptr := c.builder.CreateGEP(argstruct, []llvm.Value{
			llvm.ConstInt(llvm.Int32Type(), 0, false),
			llvm.ConstInt(llvm.Int32Type(), uint64(i+nctx), false)}, "")
		ptrtyp := types.NewPointer(args[i].Type())
		args[i] = c.NewValue(argptr, ptrtyp).makePointee()
	}

	// Extract the function pointer.
	// TODO if function is a global, elide.
	fnval = llvm.Undef(fnval.Type())
	fnptrptr := c.builder.CreateGEP(argstruct, []llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), 0, false),
		llvm.ConstInt(llvm.Int32Type(), 0, false)}, "")
	fnptr = c.builder.CreateLoad(fnptrptr, "")
	fnval = c.builder.CreateInsertValue(fnval, fnptr, 0, "")
	if nctx > 1 {
		ctxptr := c.builder.CreateGEP(argstruct, []llvm.Value{
			llvm.ConstInt(llvm.Int32Type(), 0, false),
			llvm.ConstInt(llvm.Int32Type(), 1, false)}, "")
		ctx = c.builder.CreateLoad(ctxptr, "")
		fnval = c.builder.CreateInsertValue(fnval, ctx, 1, "")
		fn = c.NewValue(fnval, fn.Type())
	}
	c.createCall(fn, args, dotdotdot, false)

	// Indirect function calls' return values are always ignored.
	c.builder.CreateRetVoid()
	c.builder.SetInsertPointAtEnd(currblock)

	fnval = llvm.Undef(c.types.ToLLVM(nilarytyp))
	indirectfn = c.builder.CreateBitCast(indirectfn, fnval.Type().StructElementTypes()[0], "")
	fnval = c.builder.CreateInsertValue(fnval, indirectfn, 0, "")
	fnval = c.builder.CreateInsertValue(fnval, i8argstruct, 1, "")
	fn = c.NewValue(fnval, nilarytyp)
	return fn
}
