// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.exp/go/types"
	"fmt"
	"github.com/axw/gollvm/llvm"
	"go/ast"
	"go/scanner"
	"go/token"
	"reflect"
)

func (c *compiler) makeFunc(ident *ast.Ident, ftyp *types.Signature) *LLVMValue {
	fname := ident.String()
	if ftyp.Recv == nil && fname == "init" {
		// Make "init" functions anonymous.
		fname = ""
	} else {
		var pkgname string
		if ftyp.Recv != nil {
			var recvname string
			switch recvtyp := ftyp.Recv.Type.(type) {
			case *types.Pointer:
				if named, ok := recvtyp.Base.(*types.NamedType); ok {
					recvname = "*" + named.Obj.Name
					pkgname = c.objectdata[named.Obj].Package.Path
				}
			case *types.NamedType:
				named := recvtyp
				recvname = named.Obj.Name
				pkgname = c.objectdata[named.Obj].Package.Path
			}

			if recvname != "" {
				fname = fmt.Sprintf("%s.%s", recvname, fname)
			} else {
				// If the receiver is an unnamed struct, we're
				// synthesising a method for an unnamed struct
				// type. There's no meaningful name to give the
				// function, so leave it up to LLVM.
				fname = ""
			}
		} else {
			obj := c.objects[ident]
			pkgname = c.objectdata[obj].Package.Path
		}
		if fname != "" {
			fname = pkgname + "." + fname
		}
	}

	// gcimporter may produce multiple AST objects for the same function.
	llvmftyp := c.types.ToLLVM(ftyp)
	var fn llvm.Value
	if fname != "" {
		fn = c.module.Module.NamedFunction(fname)
	}
	if fn.IsNil() {
		llvmfptrtyp := llvmftyp.StructElementTypes()[0].ElementType()
		fn = llvm.AddFunction(c.module.Module, fname, llvmfptrtyp)
	}
	fn = llvm.ConstInsertValue(llvm.ConstNull(llvmftyp), fn, []uint32{0})
	return c.NewValue(fn, ftyp)
}

// promoteStackVar takes a stack variable Value, and promotes it to the heap,
// replacing all uses of the stack-allocated value in the process.
func (stackvar *LLVMValue) promoteStackVar() {
	c := stackvar.compiler
	stackptrval := stackvar.pointer.value

	currblock := c.builder.GetInsertBlock()
	defer c.builder.SetInsertPointAtEnd(currblock)
	c.builder.SetInsertPointBefore(stackptrval)

	typ := stackptrval.Type().ElementType()
	heapptrval := c.createTypeMalloc(typ)
	heapptrval.SetName(stackptrval.Name())
	stackptrval.ReplaceAllUsesWith(heapptrval)
	stackvar.pointer.value = heapptrval
	stackvar.stack = nil
}

// buildFunction takes a function Value, a list of parameters, and a body,
// and generates code for the function.
func (c *compiler) buildFunction(f *LLVMValue, context, params, results []*types.Var, body *ast.BlockStmt) {
	defer c.builder.SetInsertPointAtEnd(c.builder.GetInsertBlock())
	llvm_fn := llvm.ConstExtractValue(f.LLVMValue(), []uint32{0})
	entry := llvm.AddBasicBlock(llvm_fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)

	// For closures, context is the captured context values.
	var paramoffset int
	if context != nil {
		paramoffset++

		// Store the existing values. We're going to temporarily
		// replace the values with offsets into the context param.
		oldvalues := make([]*LLVMValue, len(context))
		for i, v := range context {
			oldvalues[i] = c.objectdata[v].Value
		}
		defer func() {
			for i, v := range context {
				c.objectdata[v].Value = oldvalues[i]
			}
		}()

		// The context parameter is a pointer to a struct
		// whose elements are pointers to captured values.
		arg0 := llvm_fn.Param(0)
		for i, v := range context {
			argptr := c.builder.CreateStructGEP(arg0, i, "")
			argptr = c.builder.CreateLoad(argptr, "")
			ptrtyp := oldvalues[i].pointer.Type()
			newvalue := c.NewValue(argptr, ptrtyp)
			c.objectdata[v].Value = newvalue.makePointee()
		}
	}

	// Bind receiver, arguments and return values to their
	// identifiers/objects. We'll store each parameter on the stack so
	// they're addressable.
	for i, v := range params {
		if v.Name != "" {
			value := llvm_fn.Param(i + paramoffset)
			typ := v.Type
			stackvalue := c.builder.CreateAlloca(c.types.ToLLVM(typ), v.Name)
			c.builder.CreateStore(value, stackvalue)
			ptrvalue := c.NewValue(stackvalue, &types.Pointer{Base: typ})
			stackvar := ptrvalue.makePointee()
			stackvar.stack = f
			c.objectdata[v].Value = stackvar
		}
	}

	// Allocate space on the stack for named results.
	for _, v := range results {
		if v.Name != "" {
			typ := v.Type
			llvmtyp := c.types.ToLLVM(typ)
			stackptr := c.builder.CreateAlloca(llvmtyp, v.Name)
			c.builder.CreateStore(llvm.ConstNull(llvmtyp), stackptr)
			ptrvalue := c.NewValue(stackptr, &types.Pointer{Base: typ})
			stackvar := ptrvalue.makePointee()
			stackvar.stack = f
			c.objectdata[v].Value = stackvar
		}
	}

	c.functions.push(&function{LLVMValue: f, results: results})
	c.VisitBlockStmt(body, false)
	funcstate := c.functions.pop()

	// If there are no results, then "return" is optional.
	if len(results) == 0 {
		// Use GetInsertBlock rather than LastBasicBlock, since the
		// last basic block might actually be a "defer" block.
		last := c.builder.GetInsertBlock()
		if in := last.LastInstruction(); in.IsNil() || in.IsATerminatorInst().IsNil() {
			c.builder.SetInsertPointAtEnd(last)
			if funcstate.deferblock.IsNil() {
				c.builder.CreateRetVoid()
			} else {
				c.builder.CreateBr(funcstate.deferblock)
			}
		}
	}
}

func (c *compiler) VisitFuncDecl(f *ast.FuncDecl) Value {
	fn := c.Resolve(f.Name).(*LLVMValue)
	attributes := parseAttributes(f.Doc)
	for _, attr := range attributes {
		attr.Apply(fn)
	}
	if f.Body == nil {
		return fn
	}

	var paramVars []*types.Var
	ftyp := fn.Type().(*types.Signature)
	if ftyp.Recv != nil {
		paramVars = append(paramVars, ftyp.Recv)
	}
	if ftyp.Params != nil {
		paramVars = append(paramVars, ftyp.Params...)
	}
	c.buildFunction(fn, nil, paramVars, ftyp.Results[:], f.Body)

	if f.Recv == nil && f.Name.Name == "init" {
		// Is it an 'init' function? Then record it.
		fnptr := llvm.ConstExtractValue(fn.value, []uint32{0})
		c.initfuncs = append(c.initfuncs, fnptr)
	}
	return fn
}

// Create a constructor function which initialises a global.
// TODO collapse all global inits into one init function?
func (c *compiler) createGlobals(idents []*ast.Ident, values []ast.Expr, pkg string) {
	globals := make([]*LLVMValue, len(idents))
	for i, ident := range idents {
		if ident.Name != "_" {
			t := c.objects[ident].GetType()
			llvmtyp := c.types.ToLLVM(t)
			gv := llvm.AddGlobal(c.module.Module, llvmtyp, pkg+"."+ident.Name)
			g := c.NewValue(gv, &types.Pointer{Base: t}).makePointee()
			globals[i] = g
			c.objectdata[c.objects[ident]].Value = g
		}
	}

	if len(values) == 0 {
		for _, g := range globals {
			if g != nil {
				typ := g.pointer.value.Type().ElementType()
				initializer := llvm.ConstNull(typ)
				g.pointer.value.SetInitializer(initializer)
			}
		}
		return
	} else if len(values) == len(idents) {
		// Non-compound. Initialise global variables with constant
		// values (if any). If all expressions are constant, return
		// immediately after, to avoid the unnecessary function
		// below.
		allconst := true
		for i, expr := range values {
			constinfo := c.types.expr[expr]
			if constinfo.Value != nil {
				if globals[i] != nil {
					gv := globals[i].pointer.value
					value := c.VisitExpr(expr)
					value = value.Convert(globals[i].Type())
					gv.SetInitializer(value.LLVMValue())
				}
			} else {
				allconst = false
			}
		}
		if allconst {
			return
		}
	}

	// There are non-const expressions, so we must create an init()
	// function to evaluate the expressions and initialise the globals.
	if block := c.builder.GetInsertBlock(); !block.IsNil() {
		defer c.builder.SetInsertPointAtEnd(block)
	}
	llvmfntype := llvm.FunctionType(llvm.VoidType(), nil, false)
	fn := llvm.AddFunction(c.module.Module, "", llvmfntype)
	entry := llvm.AddBasicBlock(fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)

	if len(values) == 1 && len(idents) > 1 {
		values := c.destructureExpr(values[0])
		for i, v := range values {
			if globals[i] != nil {
				//v := values[i].Convert(ident.Obj.Type.(types.Type))
				gv := globals[i].pointer.value
				gv.SetInitializer(llvm.Undef(gv.Type().ElementType()))
				c.builder.CreateStore(v.LLVMValue(), gv)
			}
		}
	} else {
		for i, expr := range values {
			constval := c.types.expr[expr].Value
			if constval == nil {
				// Must evaluate regardless of whether value is
				// assigned, in event of side-effects.
				v := c.VisitExpr(expr)
				if globals[i] != nil {
					gv := globals[i].pointer.value
					gv.SetInitializer(llvm.Undef(gv.Type().ElementType()))
					v = v.Convert(globals[i].Type())
					c.builder.CreateStore(v.LLVMValue(), gv)
				}
			}
		}
	}
	c.builder.CreateRetVoid()
	c.varinitfuncs = append(c.varinitfuncs, fn)
}

func (c *compiler) VisitValueSpec(valspec *ast.ValueSpec) {
	// Check if the value-spec has already been visited (referenced
	// before definition visited.)
	for _, name := range valspec.Names {
		if name != nil && name.Name != "_" {
			obj := c.objects[name]
			if c.objectdata[obj].Value != nil {
				return
			}
		}
	}

	if len(valspec.Values) == len(valspec.Names) {
		for i, name := range valspec.Names {
			if name.Name == "" {
				continue
			}
			typ := c.objects[name].GetType()
			c.convertUntyped(valspec.Values[i], typ)
		}
	}

	// If the ValueSpec exists at the package level, create globals.
	if obj, ok := c.objects[valspec.Names[0]]; ok {
		if c.pkg.Scope.Lookup(valspec.Names[0].Name) == obj {
			c.createGlobals(valspec.Names, valspec.Values, c.pkg.Path)
			return
		}
	}

	var values []Value
	if len(valspec.Values) == 1 && len(valspec.Names) > 1 {
		values = c.destructureExpr(valspec.Values[0])
	} else if len(valspec.Values) > 0 {
		values = make([]Value, len(valspec.Names))
		for i, x := range valspec.Values {
			values[i] = c.VisitExpr(x)
		}
	}

	for i, name := range valspec.Names {
		if name.Name == "_" {
			continue
		}

		// The variable should be allocated on the stack if it's
		// declared inside a function.
		//
		// FIXME currently allocating all variables on the heap.
		// Change this to allocate on the stack, and perform
		// escape analysis to determine whether to promote.

		obj := c.objects[name]
		typ := obj.GetType()
		llvmtyp := c.types.ToLLVM(typ)
		ptr := c.createTypeMalloc(llvmtyp)
		if values != nil && values[i] != nil {
			// FIXME we need to revisit how aggregate types
			// are initialised/copied/etc. A CreateStore will
			// try to do everything in registers, which is
			// going to hurt when the aggregate is large.
			llvmInit := values[i].Convert(typ).LLVMValue()
			c.builder.CreateStore(llvmInit, ptr)
		}
		stackvar := c.NewValue(ptr, &types.Pointer{Base: typ}).makePointee()
		stackvar.stack = c.functions.top().LLVMValue
		c.objectdata[c.objects[name]].Value = stackvar
	}
}

func (c *compiler) VisitGenDecl(decl *ast.GenDecl) {
	switch decl.Tok {
	case token.IMPORT:
		// Already handled in type-checking.
		break
	case token.TYPE:
		// Export runtime type information.
		for _, spec := range decl.Specs {
			typspec := spec.(*ast.TypeSpec)
			typ := c.objects[typspec.Name].GetType()
			c.types.ToRuntime(typ)
		}
	case token.CONST:
		// Nothing to do; constants are evaluated by go/types.
		// They are converted to LLVM constant values at the
		// site of use.
	case token.VAR:
		// Global variable attributes
		// TODO only parse attributes for package-level var's.
		attributes := parseAttributes(decl.Doc)
		for _, spec := range decl.Specs {
			valspec, _ := spec.(*ast.ValueSpec)
			c.VisitValueSpec(valspec)
			for _, attr := range attributes {
				for _, name := range valspec.Names {
					obj := c.objects[name]
					attr.Apply(c.objectdata[obj].Value)
				}
			}
		}
	}
}

func (c *compiler) VisitDecl(decl ast.Decl) Value {
	// This is temporary. We'll return errors later, rather than panicking.
	if c.Logger != nil {
		c.Logger.Println("Compile declaration:", c.fileset.Position(decl.Pos()))
	}
	defer func() {
		if e := recover(); e != nil {
			elist := new(scanner.ErrorList)
			elist.Add(c.fileset.Position(decl.Pos()), fmt.Sprint(e))
			panic(elist)
		}
	}()

	switch x := decl.(type) {
	case *ast.FuncDecl:
		return c.VisitFuncDecl(x)
	case *ast.GenDecl:
		c.VisitGenDecl(x)
		return nil
	}
	panic(fmt.Sprintf("Unhandled decl (%s) at %s\n",
		reflect.TypeOf(decl),
		c.fileset.Position(decl.Pos())))
}
