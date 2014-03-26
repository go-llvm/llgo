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
	"github.com/axw/gollvm/llvm"
)

type unit struct {
	*compiler
	pkg     *ssa.Package
	globals map[ssa.Value]llvm.Value

	// undefinedFuncs contains functions that have been resolved
	// (declared) but not defined.
	undefinedFuncs map[*ssa.Function]bool
}

func newUnit(c *compiler, pkg *ssa.Package) *unit {
	u := &unit{
		compiler:       c,
		pkg:            pkg,
		globals:        make(map[ssa.Value]llvm.Value),
		undefinedFuncs: make(map[*ssa.Function]bool),
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
			u.globals[v] = global
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
	llvmFunction := u.resolveFunctionGlobal(f)
	return u.NewValue(llvm.ConstBitCast(llvmFunction, llvm.PointerType(llvm.Int8Type(), 0)), f.Signature)
}

func (u *unit) resolveFunctionGlobal(f *ssa.Function) llvm.Value {
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
		fti := u.llvmtypes.getSignatureInfo(f.Signature)
		llvmFunction = fti.declare(u.module.Module, name)
		/*
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
		*/
		u.undefinedFuncs[f] = true
	}
	u.globals[f] = llvmFunction
	return llvmFunction
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
		unit:       u,
		blocks:     make([]llvm.BasicBlock, len(f.Blocks)),
		lastBlocks: make([]llvm.BasicBlock, len(f.Blocks)),
		env:        make(map[ssa.Value]*LLVMValue),
		tuples:     make(map[ssa.Value][]*LLVMValue),
	}

	fr.logf("Define function: %s", f.String())
	llvmFunction := fr.resolveFunctionGlobal(f)
	fti := u.llvmtypes.getSignatureInfo(f.Signature)
	delete(u.undefinedFuncs, f)
	fr.function = llvmFunction
	fr.retInf = fti.retInf

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

	prologueBlock := llvm.InsertBasicBlock(fr.blocks[0], "prologue")
	fr.builder.SetInsertPointAtEnd(prologueBlock)

	isMethod := f.Signature.Recv() != nil

	// Map parameter positions to indices. We use this
	// when processing locals to map back to parameters
	// when generating debug metadata.
	paramPos := make(map[token.Pos]int)
	for i, param := range f.Params {
		paramPos[param.Pos()] = i + paramOffset
		llparam := fti.argInfos[i].decode(llvm.GlobalContext(), fr.builder, fr.builder)
		if isMethod && i == 0 {
			if _, ok := param.Type().Underlying().(*types.Pointer); !ok {
				llparam = fr.builder.CreateBitCast(llparam, llvm.PointerType(fr.types.ToLLVM(param.Type()), 0), "")
				llparam = fr.builder.CreateLoad(llparam, "")
			}
		}
		fr.env[param] = fr.NewValue(llparam, param.Type())
	}

	// Allocate stack space for locals in the prologue block.
	for _, local := range f.Locals {
		typ := fr.llvmtypes.ToLLVM(deref(local.Type()))
		alloca := fr.builder.CreateAlloca(typ, local.Comment)
		fr.memsetZero(alloca, llvm.SizeOf(typ))
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

	var term llvm.Value
	// If the function contains any defers, we must first call
	// setjmp so we can call rundefers in response to a panic.
	// We can short-circuit the check for defers with
	// f.Recover != nil.
	if f.Recover != nil || hasDefer(f) {
		panic("setjmp unsupported")
		/*
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
		term = fr.builder.CreateBr(recoverBlock)
		*/
	} else {
		term = fr.builder.CreateBr(fr.blocks[0])
	}
	fr.allocaBuilder.SetInsertPointBefore(term)

	for _, block := range f.DomPreorder() {
		fr.translateBlock(block, fr.blocks[block.Index])
	}

	fr.fixupPhis()
}

type pendingPhi struct {
	ssa  *ssa.Phi
	llvm llvm.Value
}

type frame struct {
	*unit
	function   llvm.Value
	retInf     retInfo
	blocks     []llvm.BasicBlock
	lastBlocks []llvm.BasicBlock
	env        map[ssa.Value]*LLVMValue
	tuples     map[ssa.Value][]*LLVMValue
	phis       []pendingPhi
}

func (fr *frame) fixupPhis() {
	for _, phi := range fr.phis {
		values := make([]llvm.Value, len(phi.ssa.Edges))
		blocks := make([]llvm.BasicBlock, len(phi.ssa.Edges))
		block := phi.ssa.Block()
		for i, edge := range phi.ssa.Edges {
			values[i] = fr.value(edge).LLVMValue()
			blocks[i] = fr.lastBlock(block.Preds[i])
		}
		phi.llvm.AddIncoming(values, blocks)
	}
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
	fr.lastBlocks[b.Index] = fr.builder.GetInsertBlock()
}

func (fr *frame) block(b *ssa.BasicBlock) llvm.BasicBlock {
	return fr.blocks[b.Index]
}

func (fr *frame) lastBlock(b *ssa.BasicBlock) llvm.BasicBlock {
	return fr.lastBlocks[b.Index]
}

func (fr *frame) value(v ssa.Value) (result *LLVMValue) {
	switch v := v.(type) {
	case nil:
		return nil
	case *ssa.Function:
		return fr.resolveFunction(v)
	case *ssa.Const:
		return fr.NewConstValue(v.Value, v.Type())
	case *ssa.Global:
		if g, ok := fr.globals[v]; ok {
			return fr.NewValue(g, v.Type())
		}
		// Create an external global. Globals for this package are defined
		// on entry to translatePackage, and have initialisers.
		llelemtyp := fr.llvmtypes.ToLLVM(deref(v.Type()))
		llglobal := llvm.AddGlobal(fr.module.Module, llelemtyp, v.String())
		fr.globals[v] = llglobal
		return fr.NewValue(llglobal, v.Type())
	}
	if value, ok := fr.env[v]; ok {
		return value
	}

	panic("Instruction not visited yet")
}

func (fr *frame) instruction(instr ssa.Instruction) {
	fr.logf("[%T] %v @ %s\n", instr, instr, fr.pkg.Prog.Fset.Position(instr.Pos()))
	if fr.GenerateDebug {
		fr.debug.setLocation(fr.builder, instr.Pos())
	}

	switch instr := instr.(type) {
	case *ssa.Alloc:
		typ := deref(instr.Type())
		llvmtyp := fr.llvmtypes.ToLLVM(typ)
		var value llvm.Value
		if instr.Heap {
			value = fr.createTypeMalloc(typ)
			value.SetName(instr.Comment)
			value = fr.builder.CreateBitCast(value, llvm.PointerType(llvm.Int8Type(), 0), "")
			fr.env[instr] = fr.NewValue(value, instr.Type())
		} else {
			value = fr.env[instr].LLVMValue()
		}
		fr.memsetZero(value, llvm.SizeOf(llvmtyp))

	case *ssa.BinOp:
		lhs, rhs := fr.value(instr.X), fr.value(instr.Y)
		fr.env[instr] = fr.binaryOp(lhs, instr.Op, rhs).(*LLVMValue)

	case *ssa.Call:
		tuple := fr.callInstruction(instr)
		if len(tuple) == 1 {
			fr.env[instr] = tuple[0]
		} else {
			fr.tuples[instr] = tuple
		}

	case *ssa.ChangeInterface:
		x := fr.value(instr.X)
		// The source type must be a non-empty interface,
		// as ChangeInterface cannot fail (E2I may fail).
		if instr.Type().Underlying().(*types.Interface).NumMethods() > 0 {
			// TODO(axw) optimisation for I2I case where we
			// know statically the methods to carry over.
			x = fr.changeInterface(x, instr.Type(), false)
		} else {
			x = fr.convertI2E(x)
		}
		fr.env[instr] = x

	case *ssa.ChangeType:
		value := fr.value(instr.X).LLVMValue()
		if _, ok := instr.Type().Underlying().(*types.Pointer); ok {
			value = fr.builder.CreateBitCast(value, fr.llvmtypes.ToLLVM(instr.Type()), "")
		}
		fr.env[instr] = fr.NewValue(value, instr.Type())

	case *ssa.Convert:
		v := fr.value(instr.X)
		fr.env[instr] = fr.convert(v, instr.Type()).(*LLVMValue)

	//case *ssa.DebugRef:

	case *ssa.Defer:
		panic("defer not supported yet")
	/*
		fn, args, result := fr.prepareCall(instr)
		if result != nil {
			panic("illegal use of builtin in defer statement")
		}
		fn = fr.indirectFunction(fn, args)
		fr.createCall(fr.runtime.pushdefer, []*LLVMValue{fn})
		*/

	case *ssa.Extract:
		var elem llvm.Value
		if t, ok := fr.tuples[instr.Tuple]; ok {
			elem = t[instr.Index].LLVMValue()
		} else {
			tuple := fr.value(instr.Tuple).LLVMValue()
			elem = fr.builder.CreateExtractValue(tuple, instr.Index, instr.Name())
		}
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
		xtyp := instr.X.Type().Underlying().(*types.Pointer).Elem()
		ptrtyp := llvm.PointerType(fr.llvmtypes.ToLLVM(xtyp), 0)
		ptr = fr.builder.CreateBitCast(ptr, ptrtyp, "")
		fieldptr := fr.builder.CreateStructGEP(ptr, instr.Field, instr.Name())
		fieldptr = fr.builder.CreateBitCast(fieldptr, llvm.PointerType(llvm.Int8Type(), 0), "")
		fieldptrtyp := instr.Type()
		fr.env[instr] = fr.NewValue(fieldptr, fieldptrtyp)

	case *ssa.Go:
		fn, arg := fr.createThunk(&instr.Call)
		fr.runtime.Go.call(fr, fn, arg)

	case *ssa.If:
		cond := fr.value(instr.Cond).LLVMValue()
		block := instr.Block()
		trueBlock := fr.block(block.Succs[0])
		falseBlock := fr.block(block.Succs[1])
		cond = fr.builder.CreateTrunc(cond, llvm.Int1Type(), "")
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
		var elemtyp types.Type
		switch typ := instr.X.Type().Underlying().(type) {
		case *types.Slice:
			elemtyp = typ.Elem()
			x = fr.builder.CreateExtractValue(x, 0, "")
		case *types.Pointer: // *array
			elemtyp = typ.Elem().Underlying().(*types.Array).Elem()
		}
		ptrtyp := llvm.PointerType(fr.llvmtypes.ToLLVM(elemtyp), 0)
		x = fr.builder.CreateBitCast(x, ptrtyp, "")
		addr := fr.builder.CreateGEP(x, []llvm.Value{index}, "")
		addr = fr.builder.CreateBitCast(addr, llvm.PointerType(llvm.Int8Type(), 0), "")
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
			v, ok := fr.mapLookup(x, index)
			if instr.CommaOk {
				fr.tuples[instr] = []*LLVMValue{v, ok}
			} else {
				fr.env[instr] = v
			}
		}

	case *ssa.MakeChan:
		fr.env[instr] = fr.makeChan(instr.Type(), fr.value(instr.Size))

	case *ssa.MakeClosure:
		panic("closures not supported yet")
		/*
		fn := fr.resolveFunction(instr.Fn.(*ssa.Function))
		bindings := make([]*LLVMValue, len(instr.Bindings))
		for i, binding := range instr.Bindings {
			bindings[i] = fr.value(binding)
		}
		fr.env[instr] = fr.makeClosure(fn, bindings)
		*/

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
		fr.phis = append(fr.phis, pendingPhi{instr, phi})

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
		vals := make([]llvm.Value, len(instr.Results))
		for i, res := range instr.Results {
			vals[i] = fr.value(res).LLVMValue()
		}
		fr.retInf.encode(llvm.GlobalContext(), fr.allocaBuilder, fr.builder, vals)

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
		if instr.CommaOk {
			v, ok := fr.interfaceTypeCheck(x, instr.AssertedType)
			fr.tuples[instr] = []*LLVMValue{v, ok}
		} else {
			fr.env[instr] = fr.interfaceTypeAssert(x, instr.AssertedType)
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

func (fr *frame) callBuiltin(typ types.Type, builtin *ssa.Builtin, args []*LLVMValue) []*LLVMValue {
	switch builtin.Name() {
	case "print", "println":
		fr.printValues(builtin.Name() == "println", args...)
		return nil

	case "panic":
		panic("TODO: panic")

	case "recover":
		panic("TODO: recover")

	case "append":
		return []*LLVMValue { fr.callAppend(args[0], args[1]) }

	case "close":
		panic("TODO: close")

	case "cap":
		return []*LLVMValue { fr.callCap(args[0]) }

	case "len":
		return []*LLVMValue { fr.callLen(args[0]) }

	case "copy":
		return []*LLVMValue { fr.callCopy(args[0], args[1]) }

	case "delete":
		fr.mapDelete(args[0], args[1])
		return nil

	case "real":
		return []*LLVMValue { args[0].extractComplexComponent(0) }

	case "imag":
		return []*LLVMValue { args[0].extractComplexComponent(1) }

	case "complex":
		r := args[0].LLVMValue()
		i := args[1].LLVMValue()
		cmplx := llvm.Undef(fr.llvmtypes.ToLLVM(typ))
		cmplx = fr.builder.CreateInsertValue(cmplx, r, 0, "")
		cmplx = fr.builder.CreateInsertValue(cmplx, i, 1, "")
		return []*LLVMValue { fr.NewValue(cmplx, typ) }

	default:
		panic("unimplemented: " + builtin.Name())
	}
}

// prepareCall returns the evaluated function and arguments.
//
// For builtins that may not be used in go/defer, prepareCall
// will emits inline code. In this case, prepareCall returns
// nil for fn and args, and returns a non-nil value for result.
func (fr *frame) callInstruction(instr ssa.CallInstruction) []*LLVMValue {
	call := instr.Common()
	args := make([]*LLVMValue, len(call.Args))
	for i, arg := range call.Args {
		args[i] = fr.value(arg)
	}

	if builtin, ok := call.Value.(*ssa.Builtin); ok {
		return fr.callBuiltin(instr.Value().Type(), builtin, args)
	}

	var fn *LLVMValue
	if call.IsInvoke() {
		var recv *LLVMValue
		fn, recv = fr.interfaceMethod(fr.value(call.Value), call.Method)
		args = append([]*LLVMValue{recv}, args...)
	} else {
		fn = fr.value(call.Value)
		if recv := call.Signature().Recv(); recv != nil {
			if _, ok := recv.Type().Underlying().(*types.Pointer); !ok {
				recvalloca := fr.allocaBuilder.CreateAlloca(args[0].LLVMValue().Type(), "")
				fr.builder.CreateStore(args[0].LLVMValue(), recvalloca)
				args[0] = fr.NewValue(recvalloca, types.NewPointer(args[0].Type()))
			}
		}
	}
	return fr.createCall(fn, args)
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
