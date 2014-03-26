// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"fmt"
	"go/token"
	"sort"

	"code.google.com/p/go.tools/go/ssa"
	"code.google.com/p/go.tools/go/ssa/ssautil"
	"code.google.com/p/go.tools/go/types"
	"github.com/go-llvm/llvm"
)

type unit struct {
	*compiler
	pkg     *ssa.Package
	globals map[ssa.Value]*LLVMValue

	// undefinedFuncs contains functions that have been resolved
	// (declared) but not defined.
	undefinedFuncs map[*ssa.Function]bool

	// funcvals is a map of *ssa.Function to LLVM functions that
	// may be stored. Non-receiver functions in this map will have
	// an additional context parameter, to enable non-branching
	// calls with a pair-of-pointer function representation,
	// without forcing the additional parameter on all functions.
	funcvals map[*ssa.Function]*LLVMValue
}

func newUnit(c *compiler, pkg *ssa.Package) *unit {
	u := &unit{
		compiler:       c,
		pkg:            pkg,
		globals:        make(map[ssa.Value]*LLVMValue),
		undefinedFuncs: make(map[*ssa.Function]bool),
		funcvals:       make(map[*ssa.Function]*LLVMValue),
	}
	return u
}

// translatePackage translates an *ssa.Package into an LLVM module, and returns
// the translation unit information.
func (u *unit) translatePackage(pkg *ssa.Package) {
	// Initialize global storage.
	for _, m := range pkg.Members {
		switch v := m.(type) {
		case *ssa.Global:
			llelemtyp := u.llvmtypes.ToLLVM(deref(v.Type()))
			global := llvm.AddGlobal(u.module.Module, llelemtyp, v.String())
			global.SetInitializer(llvm.ConstNull(llelemtyp))
			u.globals[v] = u.NewValue(global, v.Type())
		}
	}

	// Define functions.
	// Sort if flag is set for deterministic behaviour (for debugging)
	functions := ssautil.AllFunctions(pkg.Prog)
	if !u.compiler.OrderedCompilation {
		for f, _ := range functions {
			u.defineFunction(f)
		}
	} else {
		fns := []*ssa.Function{}
		for f, _ := range functions {
			fns = append(fns, f)
		}
		sort.Sort(byName(fns))
		for _, f := range fns {
			u.defineFunction(f)
		}
	}

	// Define remaining functions that were resolved during
	// runtime type mapping, but not defined.
	for f, _ := range u.undefinedFuncs {
		u.defineFunction(f)
	}
}

// ResolveMethod implements MethodResolver.ResolveMethod.
func (u *unit) ResolveMethod(s *types.Selection) *LLVMValue {
	return u.resolveFunction(u.pkg.Prog.Method(s))
}

// ResolveFunc implements FuncResolver.ResolveFunc.
func (u *unit) ResolveFunc(f *types.Func) *LLVMValue {
	return u.resolveFunction(u.pkg.Prog.FuncValue(f))
}

func (u *unit) resolveFunction(f *ssa.Function) *LLVMValue {
	if v, ok := u.globals[f]; ok {
		return v
	}
	name := f.String()
	if f.Enclosing != nil {
		// Anonymous functions are not guaranteed to
		// have unique identifiers at the global scope.
		name = f.Enclosing.String() + ":" + name
	}
	// It's possible that the function already exists in the module;
	// for example, if it's a runtime intrinsic that the compiler
	// has already referenced.
	llvmFunction := u.module.Module.NamedFunction(name)
	if llvmFunction.IsNil() {
		llvmType := u.llvmtypes.ToLLVM(f.Signature)
		llvmType = llvmType.StructElementTypes()[0].ElementType()
		if len(f.FreeVars) > 0 {
			// Add an implicit first argument.
			returnType := llvmType.ReturnType()
			paramTypes := llvmType.ParamTypes()
			vararg := llvmType.IsFunctionVarArg()
			blockElementTypes := make([]llvm.Type, len(f.FreeVars))
			for i, fv := range f.FreeVars {
				blockElementTypes[i] = u.llvmtypes.ToLLVM(fv.Type())
			}
			blockType := llvm.StructType(blockElementTypes, false)
			blockPtrType := llvm.PointerType(blockType, 0)
			paramTypes = append([]llvm.Type{blockPtrType}, paramTypes...)
			llvmType = llvm.FunctionType(returnType, paramTypes, vararg)
		}
		llvmFunction = llvm.AddFunction(u.module.Module, name, llvmType)
		if f.Enclosing != nil {
			llvmFunction.SetLinkage(llvm.PrivateLinkage)
		}
		u.undefinedFuncs[f] = true
	}
	v := u.NewValue(llvmFunction, f.Signature)
	u.globals[f] = v
	return v
}

func (u *unit) defineFunction(f *ssa.Function) {
	// Nothing to do for functions without bodies.
	if len(f.Blocks) == 0 {
		return
	}

	// Only define functions from this package.
	if f.Pkg == nil {
		if r := f.Signature.Recv(); r != nil {
			if r.Pkg() != nil && r.Pkg() != u.pkg.Object {
				return
			} else if named, ok := r.Type().(*types.Named); ok && named.Obj().Parent() == types.Universe {
				// This condition is true iff f is error.Error.
				if u.pkg.Object.Path() != "runtime" {
					return
				}
			}
		}
	} else if f.Pkg != u.pkg {
		return
	}

	fr := frame{
		unit:   u,
		blocks: make([]llvm.BasicBlock, len(f.Blocks)),
		env:    make(map[ssa.Value]*LLVMValue),
	}

	fr.logf("Define function: %s", f.String())
	llvmFunction := fr.resolveFunction(f).LLVMValue()
	delete(u.undefinedFuncs, f)

	// Push the function onto the debug context.
	// TODO(axw) create a fake CU for synthetic functions
	if u.GenerateDebug && f.Synthetic == "" {
		u.debug.pushFunctionContext(llvmFunction, f.Signature, f.Pos())
		defer u.debug.popFunctionContext()
		u.debug.setLocation(u.builder, f.Pos())
	}

	// Functions that call recover must not be inlined, or we
	// can't tell whether the recover call is valid at runtime.
	if f.Recover != nil {
		llvmFunction.AddFunctionAttr(llvm.NoInlineAttribute)
	}

	for i, block := range f.Blocks {
		fr.blocks[i] = llvm.AddBasicBlock(llvmFunction, fmt.Sprintf(".%d.%s", i, block.Comment))
	}
	fr.builder.SetInsertPointAtEnd(fr.blocks[0])

	var paramOffset int
	if len(f.FreeVars) > 0 {
		// Extract captures from the first implicit parameter.
		arg0 := llvmFunction.Param(0)
		for i, fv := range f.FreeVars {
			addressPtr := fr.builder.CreateStructGEP(arg0, i, "")
			address := fr.builder.CreateLoad(addressPtr, "")
			fr.env[fv] = fr.NewValue(address, fv.Type())
		}
		paramOffset++
	}
	// Map parameter positions to indices. We use this
	// when processing locals to map back to parameters
	// when generating debug metadata.
	paramPos := make(map[token.Pos]int)
	for i, param := range f.Params {
		paramPos[param.Pos()] = i + paramOffset
		llparam := llvmFunction.Param(i + paramOffset)
		fr.env[param] = fr.NewValue(llparam, param.Type())
	}

	// Allocate stack space for locals in the prologue block.
	prologueBlock := llvm.InsertBasicBlock(fr.blocks[0], "prologue")
	fr.builder.SetInsertPointAtEnd(prologueBlock)
	for _, local := range f.Locals {
		typ := fr.llvmtypes.ToLLVM(deref(local.Type()))
		alloca := fr.builder.CreateAlloca(typ, local.Comment)
		u.memsetZero(alloca, llvm.SizeOf(typ))
		value := fr.NewValue(alloca, local.Type())
		fr.env[local] = value
		if fr.GenerateDebug {
			paramIndex, ok := paramPos[local.Pos()]
			if !ok {
				paramIndex = -1
			}
			fr.debug.declare(fr.builder, local, alloca, paramIndex)
		}
	}

	// Move any allocs relating to named results from the entry block
	// to the prologue block, so they dominate the rundefers and recover
	// blocks.
	//
	// TODO(axw) ask adonovan for a cleaner way of doing this, e.g.
	// have ssa generate an entry block that defines Allocs and related
	// stores, and then a separate block for function body instructions.
	if f.Synthetic == "" {
		if results := f.Signature.Results(); results != nil {
			for i := 0; i < results.Len(); i++ {
				result := results.At(i)
				if result.Name() == "" {
					break
				}
				for i, instr := range f.Blocks[0].Instrs {
					if instr, ok := instr.(*ssa.Alloc); ok && instr.Heap && instr.Pos() == result.Pos() {
						fr.instruction(instr)
						instrs := f.Blocks[0].Instrs
						instrs = append(instrs[:i], instrs[i+1:]...)
						f.Blocks[0].Instrs = instrs
						break
					}
				}
			}
		}
	}

	// If the function contains any defers, we must first call
	// setjmp so we can call rundefers in response to a panic.
	// We can short-circuit the check for defers with
	// f.Recover != nil.
	if f.Recover != nil || hasDefer(f) {
		rdblock := llvm.AddBasicBlock(llvmFunction, "rundefers")
		defers := fr.builder.CreateAlloca(fr.runtime.defers.llvm, "")
		fr.builder.CreateCall(fr.runtime.initdefers.LLVMValue(), []llvm.Value{defers}, "")
		jb := fr.builder.CreateStructGEP(defers, 0, "")
		jb = fr.builder.CreateBitCast(jb, llvm.PointerType(llvm.Int8Type(), 0), "")
		result := fr.builder.CreateCall(fr.runtime.setjmp.LLVMValue(), []llvm.Value{jb}, "")
		result = fr.builder.CreateIsNotNull(result, "")
		fr.builder.CreateCondBr(result, rdblock, fr.blocks[0])
		// We'll only get here via a panic, which must either be
		// recovered or continue panicking up the stack without
		// returning from "rundefers". The recover block may be
		// nil even if we can recover, in which case we just need
		// to return the zero value for each result (if any).
		var recoverBlock llvm.BasicBlock
		if f.Recover != nil {
			recoverBlock = fr.block(f.Recover)
		} else {
			recoverBlock = llvm.AddBasicBlock(llvmFunction, "recover")
			fr.builder.SetInsertPointAtEnd(recoverBlock)
			var nresults int
			results := f.Signature.Results()
			if results != nil {
				nresults = results.Len()
			}
			switch nresults {
			case 0:
				fr.builder.CreateRetVoid()
			case 1:
				fr.builder.CreateRet(llvm.ConstNull(fr.llvmtypes.ToLLVM(results.At(0).Type())))
			default:
				values := make([]llvm.Value, nresults)
				for i := range values {
					values[i] = llvm.ConstNull(fr.llvmtypes.ToLLVM(results.At(i).Type()))
				}
				fr.builder.CreateAggregateRet(values)
			}
		}
		fr.builder.SetInsertPointAtEnd(rdblock)
		fr.builder.CreateCall(fr.runtime.rundefers.LLVMValue(), nil, "")
		fr.builder.CreateBr(recoverBlock)
	} else {
		fr.builder.CreateBr(fr.blocks[0])
	}

	for i, block := range f.Blocks {
		fr.translateBlock(block, fr.blocks[i])
	}
}

type frame struct {
	*unit
	blocks    []llvm.BasicBlock
	backpatch map[ssa.Value]*LLVMValue
	env       map[ssa.Value]*LLVMValue
}

func (fr *frame) translateBlock(b *ssa.BasicBlock, llb llvm.BasicBlock) {
	if fr.GenerateDebug {
		fr.debug.pushBlockContext(b.Instrs[0].Pos())
		defer fr.debug.popBlockContext()
	}
	fr.builder.SetInsertPointAtEnd(llb)
	for _, instr := range b.Instrs {
		fr.instruction(instr)
	}
}

func (fr *frame) block(b *ssa.BasicBlock) llvm.BasicBlock {
	return fr.blocks[b.Index]
}

func (fr *frame) value(v ssa.Value) (result *LLVMValue) {
	switch v := v.(type) {
	case nil:
		return nil
	case *ssa.Function:
		result, ok := fr.funcvals[v]
		if ok {
			return result
		}
		// fr.globals[v] has the function in raw pointer form;
		// we must convert it to <f,ctx> form. If the function
		// does not have a receiver, then create a wrapper
		// function that has an additional "context" parameter.
		f := fr.resolveFunction(v)
		if v.Signature.Recv() == nil && len(v.FreeVars) == 0 {
			f = contextFunction(fr.compiler, f)
		}
		pair := llvm.ConstNull(fr.llvmtypes.ToLLVM(f.Type()))
		fnptr := llvm.ConstBitCast(f.LLVMValue(), pair.Type().StructElementTypes()[0])
		pair = llvm.ConstInsertValue(pair, fnptr, []uint32{0})
		result = fr.NewValue(pair, f.Type())
		fr.funcvals[v] = result
		return result
	case *ssa.Const:
		return fr.NewConstValue(v.Value, v.Type())
	case *ssa.Global:
		if g, ok := fr.globals[v]; ok {
			return g
		}
		// Create an external global. Globals for this package are defined
		// on entry to translatePackage, and have initialisers.
		llelemtyp := fr.llvmtypes.ToLLVM(deref(v.Type()))
		llglobal := llvm.AddGlobal(fr.module.Module, llelemtyp, v.String())
		global := fr.NewValue(llglobal, v.Type())
		fr.globals[v] = global
		return global
	}
	if value, ok := fr.env[v]; ok {
		return value
	}

	// Instructions are not necessarily visited before they are used (e.g. Phi
	// edges) so we must "backpatch": create a value with the resultant type,
	// and then replace it when we visit the instruction.
	if b, ok := fr.backpatch[v]; ok {
		return b
	}
	if fr.backpatch == nil {
		fr.backpatch = make(map[ssa.Value]*LLVMValue)
	}
	// Note: we must not create a constant here (e.g. Undef/ConstNull), as
	// it is not permissible to replace a constant with a non-constant.
	// We must create the value in its own standalone basic block, so we can
	// dispose of it after replacing.
	currBlock := fr.builder.GetInsertBlock()
	fr.builder.SetInsertPointAtEnd(llvm.AddBasicBlock(currBlock.Parent(), ""))
	placeholder := fr.compiler.builder.CreatePHI(fr.llvmtypes.ToLLVM(v.Type()), "")
	fr.builder.SetInsertPointAtEnd(currBlock)
	value := fr.NewValue(placeholder, v.Type())
	fr.backpatch[v] = value
	return value
}

// backpatcher returns, if necessary, a function that may
// be called to backpatch a placeholder value; if backpatching
// is unnecessary, the backpatcher returns nil.
//
// When the returned function is called, it is expected that
// fr.env[v] contains the value to backpatch.
func (fr *frame) backpatcher(v ssa.Value) func() {
	b := fr.backpatch[v]
	if b == nil {
		return nil
	}
	return func() {
		b.LLVMValue().ReplaceAllUsesWith(fr.env[v].LLVMValue())
		b.LLVMValue().InstructionParent().EraseFromParent()
		delete(fr.backpatch, v)
	}
}

func (fr *frame) instruction(instr ssa.Instruction) {
	fr.logf("[%T] %v @ %s\n", instr, instr, fr.pkg.Prog.Fset.Position(instr.Pos()))
	if fr.GenerateDebug {
		fr.debug.setLocation(fr.builder, instr.Pos())
	}

	// Check if we'll need to backpatch; see comment
	// in fr.value().
	if v, ok := instr.(ssa.Value); ok {
		if b := fr.backpatcher(v); b != nil {
			defer b()
		}
	}

	switch instr := instr.(type) {
	case *ssa.Alloc:
		typ := fr.llvmtypes.ToLLVM(deref(instr.Type()))
		var value llvm.Value
		if instr.Heap {
			value = fr.createTypeMalloc(typ)
			value.SetName(instr.Comment)
			fr.env[instr] = fr.NewValue(value, instr.Type())
		} else {
			value = fr.env[instr].LLVMValue()
		}
		fr.memsetZero(value, llvm.SizeOf(typ))

	case *ssa.BinOp:
		lhs, rhs := fr.value(instr.X), fr.value(instr.Y)
		fr.env[instr] = lhs.BinaryOp(instr.Op, rhs).(*LLVMValue)

	case *ssa.Call:
		fn, args, result := fr.prepareCall(instr)
		// Some builtins may only be used immediately, and not
		// deferred; in this case, "fn" will be nil, and result
		// may be non-nil (it will be nil for builtins without
		// results.)
		if fn == nil {
			if result != nil {
				fr.env[instr] = result
			}
		} else {
			result = fr.createCall(fn, args)
			fr.env[instr] = result
		}

	case *ssa.ChangeInterface:
		x := fr.value(instr.X)
		// The source type must be a non-empty interface,
		// as ChangeInterface cannot fail (E2I may fail).
		if instr.Type().Underlying().(*types.Interface).NumMethods() > 0 {
			// TODO(axw) optimisation for I2I case where we
			// know statically the methods to carry over.
			x = x.convertI2E()
			x, _ = x.convertE2I(instr.Type())
		} else {
			x = x.convertI2E()
			x = fr.NewValue(x.LLVMValue(), instr.Type())
		}
		fr.env[instr] = x

	case *ssa.ChangeType:
		value := fr.value(instr.X).LLVMValue()
		if _, ok := instr.Type().Underlying().(*types.Pointer); ok {
			value = fr.builder.CreateBitCast(value, fr.llvmtypes.ToLLVM(instr.Type()), "")
		}
		v := fr.NewValue(value, instr.Type())
		if _, ok := instr.X.(*ssa.Phi); ok {
			v = phiValue(fr.compiler, v)
		}
		fr.env[instr] = v

	case *ssa.Convert:
		v := fr.value(instr.X)
		if _, ok := instr.X.(*ssa.Phi); ok {
			v = phiValue(fr.compiler, v)
		}
		fr.env[instr] = v.Convert(instr.Type()).(*LLVMValue)

	//case *ssa.DebugRef:

	case *ssa.Defer:
		fn, args, result := fr.prepareCall(instr)
		if result != nil {
			panic("illegal use of builtin in defer statement")
		}
		fn = fr.indirectFunction(fn, args)
		fr.createCall(fr.runtime.pushdefer, []*LLVMValue{fn})

	case *ssa.Extract:
		tuple := fr.value(instr.Tuple).LLVMValue()
		elem := fr.builder.CreateExtractValue(tuple, instr.Index, instr.Name())
		elemtyp := instr.Type()
		fr.env[instr] = fr.NewValue(elem, elemtyp)

	case *ssa.Field:
		value := fr.value(instr.X).LLVMValue()
		field := fr.builder.CreateExtractValue(value, instr.Field, instr.Name())
		fieldtyp := instr.Type()
		fr.env[instr] = fr.NewValue(field, fieldtyp)

	case *ssa.FieldAddr:
		// TODO: implement nil check and panic.
		// TODO: combine a chain of {Field,Index}Addrs into a single GEP.
		ptr := fr.value(instr.X).LLVMValue()
		fieldptr := fr.builder.CreateStructGEP(ptr, instr.Field, instr.Name())
		fieldptrtyp := instr.Type()
		fr.env[instr] = fr.NewValue(fieldptr, fieldptrtyp)

	case *ssa.Go:
		fn, args, result := fr.prepareCall(instr)
		if result != nil {
			panic("illegal use of builtin in go statement")
		}
		fn = fr.indirectFunction(fn, args)
		fr.createCall(fr.runtime.Go, []*LLVMValue{fn})

	case *ssa.If:
		cond := fr.value(instr.Cond).LLVMValue()
		block := instr.Block()
		trueBlock := fr.block(block.Succs[0])
		falseBlock := fr.block(block.Succs[1])
		fr.builder.CreateCondBr(cond, trueBlock, falseBlock)

	case *ssa.Index:
		// FIXME Surely we should be dealing with an
		// *array, so we can do a GEP?
		array := fr.value(instr.X).LLVMValue()
		arrayptr := fr.builder.CreateAlloca(array.Type(), "")
		fr.builder.CreateStore(array, arrayptr)
		index := fr.value(instr.Index).LLVMValue()
		zero := llvm.ConstNull(index.Type())
		addr := fr.builder.CreateGEP(arrayptr, []llvm.Value{zero, index}, "")
		fr.env[instr] = fr.NewValue(fr.builder.CreateLoad(addr, ""), instr.Type())

	case *ssa.IndexAddr:
		// TODO: implement nil-check and panic.
		// TODO: combine a chain of {Field,Index}Addrs into a single GEP.
		x := fr.value(instr.X).LLVMValue()
		index := fr.value(instr.Index).LLVMValue()
		var addr llvm.Value
		var elemtyp types.Type
		zero := llvm.ConstNull(index.Type())
		switch typ := instr.X.Type().Underlying().(type) {
		case *types.Slice:
			elemtyp = typ.Elem()
			x = fr.builder.CreateExtractValue(x, 0, "")
			addr = fr.builder.CreateGEP(x, []llvm.Value{index}, "")
		case *types.Pointer: // *array
			elemtyp = typ.Elem().Underlying().(*types.Array).Elem()
			addr = fr.builder.CreateGEP(x, []llvm.Value{zero, index}, "")
		}
		fr.env[instr] = fr.NewValue(addr, types.NewPointer(elemtyp))

	case *ssa.Jump:
		succ := instr.Block().Succs[0]
		fr.builder.CreateBr(fr.block(succ))

	case *ssa.Lookup:
		x := fr.value(instr.X)
		index := fr.value(instr.Index)
		if isString(x.Type().Underlying()) {
			fr.env[instr] = fr.stringIndex(x, index)
		} else {
			fr.env[instr] = fr.mapLookup(x, index, instr.CommaOk)
		}

	case *ssa.MakeChan:
		fr.env[instr] = fr.makeChan(instr.Type(), fr.value(instr.Size))

	case *ssa.MakeClosure:
		fn := fr.resolveFunction(instr.Fn.(*ssa.Function))
		bindings := make([]*LLVMValue, len(instr.Bindings))
		for i, binding := range instr.Bindings {
			bindings[i] = fr.value(binding)
		}
		fr.env[instr] = fr.makeClosure(fn, bindings)

	case *ssa.MakeInterface:
		receiver := fr.value(instr.X)
		fr.env[instr] = fr.makeInterface(receiver, instr.Type())

	case *ssa.MakeMap:
		fr.env[instr] = fr.makeMap(instr.Type(), fr.value(instr.Reserve))

	case *ssa.MakeSlice:
		length := fr.value(instr.Len)
		capacity := fr.value(instr.Cap)
		fr.env[instr] = fr.makeSlice(instr.Type(), length, capacity)

	case *ssa.MapUpdate:
		m := fr.value(instr.Map)
		k := fr.value(instr.Key)
		v := fr.value(instr.Value)
		fr.mapUpdate(m, k, v)

	case *ssa.Next:
		iter := fr.value(instr.Iter)
		if !instr.IsString {
			fr.env[instr] = fr.mapIterNext(iter)
			return
		}

		// String range
		//
		// We make some assumptions for now around the
		// current state of affairs in go.tools/ssa.
		//
		//  - Range's block is a predecessor of Next's.
		//      (this is currently true, but may change in the future;
		//       adonovan says he will expose the dominator tree
		//       computation in the future, which we can use here).
		//  - Next is the first non-Phi instruction in its block.
		//      (this is not strictly necessary; we can move the Phi
		//       to the top of the block, and defer the tuple creation
		//       to Extract).
		assert(instr.Iter.(*ssa.Range).Block() == instr.Block().Preds[0])
		for _, blockInstr := range instr.Block().Instrs {
			if instr == blockInstr {
				break
			}
			_, isphi := blockInstr.(*ssa.Phi)
			assert(isphi)
		}
		preds := instr.Block().Preds
		llpreds := make([]llvm.BasicBlock, len(preds))
		for i, b := range preds {
			llpreds[i] = fr.block(b)
		}
		fr.env[instr] = fr.stringIterNext(iter, llpreds)

	case *ssa.Panic:
		arg := fr.value(instr.X).LLVMValue()
		fr.builder.CreateCall(fr.runtime.panic_.LLVMValue(), []llvm.Value{arg}, "")
		fr.builder.CreateUnreachable()

	case *ssa.Phi:
		typ := instr.Type()
		phi := fr.builder.CreatePHI(fr.llvmtypes.ToLLVM(typ), instr.Comment)
		fr.env[instr] = fr.NewValue(phi, typ)
		values := make([]llvm.Value, len(instr.Edges))
		blocks := make([]llvm.BasicBlock, len(instr.Edges))
		block := instr.Block()
		for i, edge := range instr.Edges {
			values[i] = fr.value(edge).LLVMValue()
			blocks[i] = fr.block(block.Preds[i])
		}
		phi.AddIncoming(values, blocks)

	case *ssa.Range:
		x := fr.value(instr.X)
		switch x.Type().Underlying().(type) {
		case *types.Map:
			fr.env[instr] = fr.mapIterInit(x)
		case *types.Basic: // string
			fr.env[instr] = x
		default:
			panic(fmt.Sprintf("unhandled range for type %T", x.Type()))
		}

	case *ssa.Return:
		switch n := len(instr.Results); n {
		case 0:
			// https://code.google.com/p/go/issues/detail?id=7022
			if r := instr.Parent().Signature.Results(); r != nil && r.Len() > 0 {
				fr.builder.CreateUnreachable()
			} else {
				fr.builder.CreateRetVoid()
			}
		case 1:
			fr.builder.CreateRet(fr.value(instr.Results[0]).LLVMValue())
		default:
			values := make([]llvm.Value, n)
			for i, result := range instr.Results {
				values[i] = fr.value(result).LLVMValue()
			}
			fr.builder.CreateAggregateRet(values)
		}

	case *ssa.RunDefers:
		fr.builder.CreateCall(fr.runtime.rundefers.LLVMValue(), nil, "")

	case *ssa.Select:
		states := make([]selectState, len(instr.States))
		for i, state := range instr.States {
			states[i] = selectState{
				Dir:  state.Dir,
				Chan: fr.value(state.Chan),
				Send: fr.value(state.Send),
			}
		}
		fr.env[instr] = fr.chanSelect(states, instr.Blocking)

	case *ssa.Send:
		fr.chanSend(fr.value(instr.Chan), fr.value(instr.X))

	case *ssa.Slice:
		x := fr.value(instr.X)
		low := fr.value(instr.Low)
		high := fr.value(instr.High)
		fr.env[instr] = fr.slice(x, low, high)

	case *ssa.Store:
		addr := fr.value(instr.Addr).LLVMValue()
		value := fr.value(instr.Val).LLVMValue()
		// The bitcast is necessary to handle recursive pointer stores.
		addr = fr.builder.CreateBitCast(addr, llvm.PointerType(value.Type(), 0), "")
		fr.builder.CreateStore(value, addr)

	case *ssa.TypeAssert:
		x := fr.value(instr.X)
		if iface, ok := x.Type().Underlying().(*types.Interface); ok && iface.NumMethods() > 0 {
			x = x.convertI2E()
		}
		if !instr.CommaOk {
			if _, ok := instr.AssertedType.Underlying().(*types.Interface); ok {
				fr.env[instr] = x.mustConvertE2I(instr.AssertedType)
			} else {
				fr.env[instr] = x.mustConvertE2V(instr.AssertedType)
			}
		} else {
			var result, success *LLVMValue
			if _, ok := instr.AssertedType.Underlying().(*types.Interface); ok {
				result, success = x.convertE2I(instr.AssertedType)
			} else {
				result, success = x.convertE2V(instr.AssertedType)
			}
			resultval := result.LLVMValue()
			okval := success.LLVMValue()
			pairtyp := llvm.StructType([]llvm.Type{resultval.Type(), okval.Type()}, false)
			pair := llvm.Undef(pairtyp)
			pair = fr.builder.CreateInsertValue(pair, resultval, 0, "")
			pair = fr.builder.CreateInsertValue(pair, okval, 1, "")
			fr.env[instr] = fr.NewValue(pair, instr.Type())
		}

	case *ssa.UnOp:
		operand := fr.value(instr.X)
		switch instr.Op {
		case token.ARROW:
			fr.env[instr] = fr.chanRecv(operand, instr.CommaOk)
		case token.MUL:
			// The bitcast is necessary to handle recursive pointer loads.
			llptr := fr.builder.CreateBitCast(operand.LLVMValue(), llvm.PointerType(fr.llvmtypes.ToLLVM(instr.Type()), 0), "")
			fr.env[instr] = fr.NewValue(fr.builder.CreateLoad(llptr, ""), instr.Type())
		default:
			fr.env[instr] = operand.UnaryOp(instr.Op).(*LLVMValue)
		}

	default:
		panic(fmt.Sprintf("unhandled: %v", instr))
	}
}

// prepareCall returns the evaluated function and arguments.
//
// For builtins that may not be used in go/defer, prepareCall
// will emits inline code. In this case, prepareCall returns
// nil for fn and args, and returns a non-nil value for result.
func (fr *frame) prepareCall(instr ssa.CallInstruction) (fn *LLVMValue, args []*LLVMValue, result *LLVMValue) {
	call := instr.Common()
	args = make([]*LLVMValue, len(call.Args))
	for i, arg := range call.Args {
		args[i] = fr.value(arg)
	}

	if call.IsInvoke() {
		fn := fr.interfaceMethod(fr.value(call.Value), call.Method)
		return fn, args, nil
	}

	switch v := call.Value.(type) {
	case *ssa.Builtin:
		// handled below
	case *ssa.Function:
		// Function handled specially; value() will convert
		// a function to one with a context argument.
		fn = fr.resolveFunction(v)
		pair := llvm.ConstNull(fr.llvmtypes.ToLLVM(fn.Type()))
		pair = llvm.ConstInsertValue(pair, fn.LLVMValue(), []uint32{0})
		fn = fr.NewValue(pair, fn.Type())
		return fn, args, nil
	default:
		fn = fr.value(call.Value)
		return fn, args, nil
	}

	// Builtins may only be used in calls (i.e. can't be assigned),
	// and only print[ln], panic and recover may be used in go/defer.
	builtin := call.Value.(*ssa.Builtin)
	switch builtin.Name() {
	case "print", "println":
		// print/println generates a call-site specific anonymous
		// function to print the values. It's not inline because
		// print/println may be deferred.
		params := make([]*types.Var, len(call.Args))
		for i, arg := range call.Args {
			// make sure to use args[i].Type(), not call.Args[i].Type(),
			// as the evaluated expression converts untyped.
			params[i] = types.NewParam(arg.Pos(), nil, arg.Name(), args[i].Type())
		}
		sig := types.NewSignature(nil, nil, types.NewTuple(params...), nil, false)
		llfntyp := fr.llvmtypes.ToLLVM(sig)
		llfnptr := llvm.AddFunction(fr.module.Module, "", llfntyp.StructElementTypes()[0].ElementType())
		currBlock := fr.builder.GetInsertBlock()
		entry := llvm.AddBasicBlock(llfnptr, "entry")
		fr.builder.SetInsertPointAtEnd(entry)
		internalArgs := make([]Value, len(args))
		for i, arg := range args {
			internalArgs[i] = fr.NewValue(llfnptr.Param(i), arg.Type())
		}
		fr.printValues(builtin.Name() == "println", internalArgs...)
		fr.builder.CreateRetVoid()
		fr.builder.SetInsertPointAtEnd(currBlock)
		return fr.NewValue(llfnptr, sig), args, nil

	case "panic":
		panic("TODO: panic")

	case "recover":
		// TODO(axw) determine number of frames to skip in pc check
		indirect := fr.NewValue(llvm.ConstNull(llvm.Int32Type()), types.Typ[types.Int32])
		return fr.runtime.recover_, []*LLVMValue{indirect}, nil

	case "append":
		return nil, nil, fr.callAppend(args[0], args[1])

	case "close":
		return fr.runtime.chanclose, args, nil

	case "cap":
		return nil, nil, fr.callCap(args[0])

	case "len":
		return nil, nil, fr.callLen(args[0])

	case "copy":
		return nil, nil, fr.callCopy(args[0], args[1])

	case "delete":
		fr.callDelete(args[0], args[1])
		return nil, nil, nil

	case "real":
		return nil, nil, args[0].extractComplexComponent(0)

	case "imag":
		return nil, nil, args[0].extractComplexComponent(1)

	case "complex":
		r := args[0].LLVMValue()
		i := args[1].LLVMValue()
		typ := instr.Value().Type()
		cmplx := llvm.Undef(fr.llvmtypes.ToLLVM(typ))
		cmplx = fr.builder.CreateInsertValue(cmplx, r, 0, "")
		cmplx = fr.builder.CreateInsertValue(cmplx, i, 1, "")
		return nil, nil, fr.NewValue(cmplx, typ)

	default:
		panic("unimplemented: " + builtin.Name())
	}
}

func hasDefer(f *ssa.Function) bool {
	for _, b := range f.Blocks {
		for _, instr := range b.Instrs {
			if _, ok := instr.(*ssa.Defer); ok {
				return true
			}
		}
	}
	return false
}

// contextFunction creates a wrapper function that
// has the same signature as the specified function,
// but has an additional first parameter that accepts
// and ignores the function context value.
//
// contextFunction must be called with a global function
// pointer.
func contextFunction(c *compiler, f *LLVMValue) *LLVMValue {
	defer c.builder.SetInsertPointAtEnd(c.builder.GetInsertBlock())
	resultType := c.llvmtypes.ToLLVM(f.Type())
	fnptr := f.LLVMValue()
	contextType := resultType.StructElementTypes()[1]
	llfntyp := fnptr.Type().ElementType()
	llfntyp = llvm.FunctionType(
		llfntyp.ReturnType(),
		append([]llvm.Type{contextType}, llfntyp.ParamTypes()...),
		llfntyp.IsFunctionVarArg(),
	)
	wrapper := llvm.AddFunction(c.module.Module, fnptr.Name()+".ctx", llfntyp)
	wrapper.SetLinkage(llvm.PrivateLinkage)
	entry := llvm.AddBasicBlock(wrapper, "entry")
	c.builder.SetInsertPointAtEnd(entry)
	args := make([]llvm.Value, len(llfntyp.ParamTypes())-1)
	for i := range args {
		args[i] = wrapper.Param(i + 1)
	}
	result := c.builder.CreateCall(fnptr, args, "")
	switch nresults := f.Type().(*types.Signature).Results().Len(); nresults {
	case 0:
		c.builder.CreateRetVoid()
	case 1:
		c.builder.CreateRet(result)
	default:
		results := make([]llvm.Value, nresults)
		for i := range results {
			results[i] = c.builder.CreateExtractValue(result, i, "")
		}
		c.builder.CreateAggregateRet(results)
	}
	return c.NewValue(wrapper, f.Type())
}

// phiValue returns a new value with the same value and type as the given Phi.
// This is used for go.tools/ssa instructions that introduce new ssa.Values,
// but would otherwise not generate a new LLVM value.
func phiValue(c *compiler, v *LLVMValue) *LLVMValue {
	llv := v.LLVMValue()
	llv = c.builder.CreateSelect(llvm.ConstAllOnes(llvm.Int1Type()), llv, llv, "")
	return c.NewValue(llv, v.Type())
}
