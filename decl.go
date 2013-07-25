// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"fmt"
	"github.com/axw/gollvm/llvm"
	"go/ast"
	"go/scanner"
	"go/token"
	"reflect"
)

func isBlank(name string) bool {
	return name == "" || name == "_"
}

func (c *compiler) makeFunc(ident *ast.Ident, ftyp *types.Signature) *LLVMValue {
	fname := ident.String()
	if ftyp.Recv() == nil && fname == "init" {
		// Make "init" functions anonymous.
		fname = ""
	} else {
		var pkgname string
		if recv := ftyp.Recv(); recv != nil {
			var recvname string
			switch recvtyp := recv.Type().(type) {
			case *types.Pointer:
				if named, ok := recvtyp.Elem().(*types.Named); ok {
					obj := named.Obj()
					recvname = "*" + obj.Name()
					pkgname = c.objectdata[obj].Package.Path()
				}
			case *types.Named:
				named := recvtyp
				obj := named.Obj()
				recvname = obj.Name()
				pkgname = c.objectdata[obj].Package.Path()
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
			obj := c.typeinfo.Objects[ident]
			pkgname = c.objectdata[obj].Package.Path()
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
func (c *compiler) buildFunction(f *LLVMValue, context, params, results *types.Tuple, body *ast.BlockStmt) {
	if currblock := c.builder.GetInsertBlock(); !currblock.IsNil() {
		defer c.builder.SetInsertPointAtEnd(currblock)
	}
	llvm_fn := llvm.ConstExtractValue(f.LLVMValue(), []uint32{0})
	entry := llvm.AddBasicBlock(llvm_fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)

	// For closures, context is the captured context values.
	var paramoffset int
	if context != nil {
		paramoffset++

		// Store the existing values. We're going to temporarily
		// replace the values with offsets into the context param.
		oldvalues := make([]*LLVMValue, context.Len())
		for i := range oldvalues {
			v := context.At(i)
			oldvalues[i] = c.objectdata[v].Value
		}
		defer func() {
			for i := range oldvalues {
				v := context.At(i)
				c.objectdata[v].Value = oldvalues[i]
			}
		}()

		// The context parameter is a pointer to a struct
		// whose elements are pointers to captured values.
		arg0 := llvm_fn.Param(0)
		for i := range oldvalues {
			v := context.At(i)
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
	nparams := int(params.Len())
	for i := 0; i < nparams; i++ {
		v := params.At(i)
		name := v.Name()
		if !isBlank(name) {
			value := llvm_fn.Param(i + paramoffset)
			typ := v.Type()
			stackvalue := c.builder.CreateAlloca(c.types.ToLLVM(typ), name)
			c.builder.CreateStore(value, stackvalue)
			ptrvalue := c.NewValue(stackvalue, types.NewPointer(typ))
			stackvar := ptrvalue.makePointee()
			stackvar.stack = f
			c.objectdata[v].Value = stackvar
		}
	}

	funcstate := &function{LLVMValue: f, results: results}
	c.functions.push(funcstate)
	hasdefer := hasDefer(funcstate, body)

	// Allocate space on the stack for named results.
	for i := 0; i < results.Len(); i++ {
		v := results.At(i)
		name := v.Name()
		allocstack := !isBlank(name)
		if !allocstack && hasdefer {
			c.objectdata[v] = &ObjectData{}
			allocstack = true
		}
		if allocstack {
			typ := v.Type()
			llvmtyp := c.types.ToLLVM(typ)
			stackptr := c.builder.CreateAlloca(llvmtyp, name)
			c.builder.CreateStore(llvm.ConstNull(llvmtyp), stackptr)
			ptrvalue := c.NewValue(stackptr, types.NewPointer(typ))
			stackvar := ptrvalue.makePointee()
			stackvar.stack = f
			c.objectdata[v].Value = stackvar
		}
	}

	// Create the function body.
	if hasdefer {
		c.makeDeferBlock(funcstate, body)
	}
	c.VisitBlockStmt(body, false)
	c.functions.pop()

	// If the last instruction in the function is not a terminator, then
	// we either have unreachable code or a missing optional return statement
	// (the latter case is allowable only for functions without results).
	//
	// Use GetInsertBlock rather than LastBasicBlock, since the
	// last basic block might actually be a "defer" block.
	last := c.builder.GetInsertBlock()
	if in := last.LastInstruction(); in.IsNil() || in.IsATerminatorInst().IsNil() {
		c.builder.SetInsertPointAtEnd(last)
		if results.Len() == 0 {
			if funcstate.deferblock.IsNil() {
				c.builder.CreateRetVoid()
			} else {
				c.builder.CreateBr(funcstate.deferblock)
			}
		} else {
			c.builder.CreateUnreachable()
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
	if recv := ftyp.Recv(); recv != nil {
		paramVars = append(paramVars, recv)
	}
	if ftyp.Params() != nil {
		for i := 0; i < ftyp.Params().Len(); i++ {
			p := ftyp.Params().At(i)
			paramVars = append(paramVars, p)
		}
	}
	paramVarsTuple := types.NewTuple(paramVars...)
	c.buildFunction(fn, nil, paramVarsTuple, ftyp.Results(), f.Body)

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
		if !isBlank(ident.Name) {
			t := c.typeinfo.Objects[ident].Type()
			llvmtyp := c.types.ToLLVM(t)
			gv := llvm.AddGlobal(c.module.Module, llvmtyp, pkg+"."+ident.Name)
			g := c.NewValue(gv, types.NewPointer(t)).makePointee()
			globals[i] = g
			c.objectdata[c.typeinfo.Objects[ident]].Value = g
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
			if constval := c.typeinfo.Values[expr]; constval != nil {
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
			if constval := c.typeinfo.Values[expr]; constval == nil {
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
		if name != nil && !isBlank(name.Name) {
			obj := c.typeinfo.Objects[name]
			if c.objectdata[obj].Value != nil {
				return
			}
		}
	}

	if len(valspec.Values) == len(valspec.Names) {
		for i, name := range valspec.Names {
			if !isBlank(name.Name) {
				typ := c.typeinfo.Objects[name].Type()
				c.convertUntyped(valspec.Values[i], typ)
			}
		}
	}

	// If the ValueSpec exists at the package level, create globals.
	if obj, ok := c.typeinfo.Objects[valspec.Names[0]]; ok {
		if c.pkg.Scope().Lookup(valspec.Names[0].Name) == obj {
			c.createGlobals(valspec.Names, valspec.Values, c.pkg.Path())
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
		if isBlank(name.Name) {
			continue
		}

		// The variable should be allocated on the stack if it's
		// declared inside a function.
		//
		// FIXME currently allocating all variables on the heap.
		// Change this to allocate on the stack, and perform
		// escape analysis to determine whether to promote.

		obj := c.typeinfo.Objects[name]
		typ := obj.Type()
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
		stackvar := c.NewValue(ptr, types.NewPointer(typ)).makePointee()
		stackvar.stack = c.functions.top().LLVMValue
		c.objectdata[c.typeinfo.Objects[name]].Value = stackvar
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
			typ := c.typeinfo.Objects[typspec.Name].Type()
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
					obj := c.typeinfo.Objects[name]
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
