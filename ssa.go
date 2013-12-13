// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"fmt"
	"go/token"
	"sort"

	"code.google.com/p/go.tools/go/types"
	"code.google.com/p/go.tools/ssa"
	"code.google.com/p/go.tools/ssa/ssautil"
	"github.com/axw/gollvm/llvm"
)

type unit struct {
	*compiler
	pkg     *ssa.Package
	globals map[ssa.Value]*LLVMValue
}

// translatePackage translates an *ssa.Package into an LLVM module, and returns
// the translation unit information.
func (c *compiler) translatePackage(pkg *ssa.Package) *unit {
	u := &unit{
		compiler: c,
		pkg:      pkg,
		globals:  make(map[ssa.Value]*LLVMValue),
	}

	// Initialize global storage.
	for _, m := range pkg.Members {
		switch v := m.(type) {
		case *ssa.Global:
			llelemtyp := c.types.ToLLVM(deref(v.Type()))
			global := llvm.AddGlobal(c.module.Module, llelemtyp, v.String())
			global.SetInitializer(llvm.ConstNull(llelemtyp))
			u.globals[v] = c.NewValue(global, v.Type())
		}
	}

	// TODO prune off unreferenced functions from imported packages,
	// so the IR isn't littered unnecessarily.

	// Translate functions.
	functions := ssautil.AllFunctions(pkg.Prog)
	for f, _ := range functions {
		u.declareFunction(f)
	}

	// Define functions.
	// Sort if flag is set for more deterministic behavior (for debugging)
	if !c.OrderedCompilation {
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
	return u
}

// declareFunction adds a function declaration with the given name
// and type to the module.
func (u *unit) declareFunction(f *ssa.Function) {
	name := f.Name()
	// TODO use f.String() for the name, which includes path,
	// receiver and function name. Runtime type generation
	// must be changed at the same time.
	if recv := f.Signature.Recv(); recv != nil {
		// receiver name includes package
		name = fmt.Sprintf("%s.%s", recv.Type(), name)
	} else if f.Pkg != nil {
		name = fmt.Sprintf("%s.%s", f.Pkg.Object.Path(), name)
	}
	llvmType := u.types.ToLLVM(f.Signature)
	llvmType = llvmType.StructElementTypes()[0].ElementType()
	if len(f.FreeVars) > 0 {
		// Add an implicit first argument.
		returnType := llvmType.ReturnType()
		paramTypes := llvmType.ParamTypes()
		vararg := llvmType.IsFunctionVarArg()
		blockElementTypes := make([]llvm.Type, len(f.FreeVars))
		for i, fv := range f.FreeVars {
			blockElementTypes[i] = u.types.ToLLVM(fv.Type())
		}
		blockType := llvm.StructType(blockElementTypes, false)
		blockPtrType := llvm.PointerType(blockType, 0)
		paramTypes = append([]llvm.Type{blockPtrType}, paramTypes...)
		llvmType = llvm.FunctionType(returnType, paramTypes, vararg)
	}
	// It's possible that the function already exists in the module;
	// for example, if it's a runtime intrinsic that the compiler
	// has already referenced.
	llvmFunction := u.module.Module.NamedFunction(name)
	if llvmFunction.IsNil() {
		llvmFunction = llvm.AddFunction(u.module.Module, name, llvmType)
	}
	u.globals[f] = u.NewValue(llvmFunction, f.Signature)
	// Functions that call recover must not be inlined, or we
	// can't tell whether the recover call is valid at runtime.
	if f.Recover != nil {
		llvmFunction.AddFunctionAttr(llvm.NoInlineAttribute)
	}
}

func (u *unit) defineFunction(f *ssa.Function) {
	// Nothing to do for functions without bodies.
	if len(f.Blocks) == 0 {
		return
	}

	fr := frame{
		unit:   u,
		blocks: make([]llvm.BasicBlock, len(f.Blocks)),
		env:    make(map[ssa.Value]*LLVMValue),
	}

	fr.logf("Define function: %s", f.String())
	llvmFunction := u.globals[f].LLVMValue()
	for i, block := range f.Blocks {
		fr.blocks[i] = llvm.AddBasicBlock(llvmFunction, block.Comment)
	}
	if f.Recover != nil {
		fr.recoverBlock = llvm.AddBasicBlock(llvmFunction, f.Recover.Comment)
	}
	u.builder.SetInsertPointAtEnd(fr.blocks[0])

	var paramOffset int
	if len(f.FreeVars) > 0 {
		// Extract captures from the first implicit parameter.
		arg0 := llvmFunction.Param(0)
		for i, fv := range f.FreeVars {
			addressPtr := u.builder.CreateStructGEP(arg0, i, "")
			address := u.builder.CreateLoad(addressPtr, "")
			fr.env[fv] = u.NewValue(address, fv.Type())
		}
		paramOffset++
	}
	for i, param := range f.Params {
		fr.env[param] = u.NewValue(llvmFunction.Param(i+paramOffset), param.Type())
	}

	// Allocate stack space for locals in the entry block.
	for _, local := range f.Locals {
		typ := u.types.ToLLVM(deref(local.Type()))
		alloca := u.builder.CreateAlloca(typ, local.Comment)
		u.memsetZero(alloca, llvm.SizeOf(typ))
		value := fr.NewValue(alloca, local.Type())
		fr.env[local] = value
	}

	for i, block := range f.Blocks {
		fr.translateBlock(block, fr.blocks[i])
	}
	if f.Recover != nil {
		fr.translateBlock(f.Recover, fr.recoverBlock)
	}
}

type frame struct {
	*unit
	blocks       []llvm.BasicBlock
	recoverBlock llvm.BasicBlock
	backpatch    map[ssa.Value]*LLVMValue
	env          map[ssa.Value]*LLVMValue
}

func (fr *frame) translateBlock(b *ssa.BasicBlock, llb llvm.BasicBlock) {
	fr.builder.SetInsertPointAtEnd(llb)
	for _, instr := range b.Instrs {
		fr.instruction(instr)
	}
}

func (fr *frame) block(b *ssa.BasicBlock) llvm.BasicBlock {
	return fr.blocks[b.Index]
}

func (fr *frame) value(v ssa.Value) *LLVMValue {
	switch v := v.(type) {
	case nil:
		return nil
	case *ssa.Function:
		// fr.globals[v] has the function in raw pointer form;
		// we must convert it to <f,ctx> form.
		f := fr.globals[v]
		pair := llvm.ConstNull(fr.types.ToLLVM(f.Type()))
		pair = llvm.ConstInsertValue(pair, f.LLVMValue(), []uint32{0})
		return fr.NewValue(pair, f.Type())
	case *ssa.Const:
		return fr.NewConstValue(v.Value, v.Type())
	case *ssa.Global:
		return fr.globals[v]
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
	placeholder := fr.compiler.builder.CreatePHI(fr.types.ToLLVM(v.Type()), "")
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

	// Check if we'll need to backpatch; see comment
	// in fr.value().
	if v, ok := instr.(ssa.Value); ok {
		if b := fr.backpatcher(v); b != nil {
			defer b()
		}
	}

	switch instr := instr.(type) {
	case *ssa.Alloc:
		typ := fr.types.ToLLVM(deref(instr.Type()))
		var value llvm.Value
		if instr.Heap {
			value = fr.createTypeMalloc(typ)
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
		// TODO convI2I, convI2E
		lliface := llvm.ConstNull(fr.types.ToLLVM(instr.Type()))
		fr.env[instr] = fr.NewValue(lliface, instr.Type())

	case *ssa.ChangeType:
		fr.env[instr] = fr.NewValue(fr.value(instr.X).LLVMValue(), instr.Type())

	case *ssa.Convert:
		fr.env[instr] = fr.value(instr.X).Convert(instr.Type()).(*LLVMValue)

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
		fn := fr.value(instr.Fn)
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
		// TODO
		fr.builder.CreateCall(fr.runtime.llvm_trap.LLVMValue(), nil, "")
		fr.builder.CreateUnreachable()

	case *ssa.Phi:
		typ := instr.Type()
		phi := fr.builder.CreatePHI(fr.types.ToLLVM(typ), instr.Comment)
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
			fr.builder.CreateRetVoid()
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
		addr := fr.value(instr.Addr)
		value := fr.value(instr.Val)
		fr.builder.CreateStore(value.LLVMValue(), addr.LLVMValue())

	case *ssa.TypeAssert:
		x := fr.value(instr.X)
		if !instr.CommaOk {
			fr.env[instr] = x.mustConvertI2V(instr.AssertedType)
		} else {
			result, ok := x.convertI2V(instr.AssertedType)
			resultval := result.LLVMValue()
			okval := ok.LLVMValue()
			pairtyp := llvm.StructType([]llvm.Type{resultval.Type(), okval.Type()}, false)
			pair := llvm.Undef(pairtyp)
			pair = fr.builder.CreateInsertValue(pair, resultval, 0, "")
			pair = fr.builder.CreateInsertValue(pair, okval, 1, "")
			fr.env[instr] = fr.NewValue(pair, instr.Type())
		}

	case *ssa.UnOp:
		operand := fr.value(instr.X)
		if instr.Op == token.ARROW {
			fr.env[instr] = fr.chanRecv(operand, instr.CommaOk)
		} else {
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

	switch call.Value.(type) {
	case *ssa.Builtin:
		// handled below
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
		fntyp := fr.types.ToLLVM(sig).StructElementTypes()[0].ElementType()
		llvmfn := llvm.AddFunction(fr.module.Module, "", fntyp)
		currBlock := fr.builder.GetInsertBlock()
		entry := llvm.AddBasicBlock(llvmfn, "entry")
		fr.builder.SetInsertPointAtEnd(entry)
		internalArgs := make([]Value, len(args))
		for i, arg := range args {
			internalArgs[i] = fr.NewValue(llvmfn.Param(i), arg.Type())
		}
		fr.printValues(builtin.Name() == "println", internalArgs...)
		fr.builder.CreateRetVoid()
		fr.builder.SetInsertPointAtEnd(currBlock)
		fn = fr.NewValue(llvmfn, sig)
		return fn, args, nil

	case "panic":
		panic("TODO: panic")

	case "recover":
		// TODO determine number of frames to skip in pc check
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
		cmplx := llvm.Undef(fr.types.ToLLVM(typ))
		cmplx = fr.builder.CreateInsertValue(cmplx, r, 0, "")
		cmplx = fr.builder.CreateInsertValue(cmplx, i, 1, "")
		return nil, nil, fr.NewValue(cmplx, typ)

	default:
		panic("unimplemented: " + builtin.Name())
	}
}
