// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
	"go/ast"
	"go/token"
)

// llgo constants.
const (
	LLGOAuthor   = "Andrew Wilkins <axwalk@gmail.com>"
	LLGOProducer = "llgo " + LLGOVersion + " (" + LLGOAuthor + ")"
)

// Go builtin types.
var (
	int32_debug_type = &llvm.BasicTypeDescriptor{
		Name:         "int32",
		Size:         32,
		Alignment:    32,
		TypeEncoding: llvm.DW_ATE_signed,
	}
	void_debug_type = &llvm.BasicTypeDescriptor{
		Name:      "void",
		Size:      0,
		Alignment: 0,
	}
)

func (c *compiler) tollvmDebugDescriptor(t types.Type) llvm.DebugDescriptor {
	switch t := t.(type) {
	case *types.Pointer:
		return llvm.NewPointerDerivedType(c.tollvmDebugDescriptor(t.Elem()))
	case nil:
		return void_debug_type
	}
	bt := &llvm.BasicTypeDescriptor{
		Name:      c.types.TypeString(t),
		Size:      uint64(c.types.Sizeof(t) * 8),
		Alignment: uint64(c.types.Alignof(t) * 8),
	}
	if basic, ok := t.(*types.Basic); ok {
		switch bi := basic.Info(); {
		case bi&types.IsBoolean != 0:
			bt.TypeEncoding = llvm.DW_ATE_boolean
		case bi&types.IsUnsigned != 0:
			bt.TypeEncoding = llvm.DW_ATE_unsigned
		case bi&types.IsInteger != 0:
			bt.TypeEncoding = llvm.DW_ATE_signed
		case bi&types.IsFloat != 0:
			bt.TypeEncoding = llvm.DW_ATE_float
		}
	}
	return bt
}

func (c *compiler) pushDebugContext(d llvm.DebugDescriptor) {
	c.debug_context = append(c.debug_context, d)
}

func (c *compiler) popDebugContext() {
	c.debug_context = c.debug_context[:len(c.debug_context)-1]
}

func (c *compiler) currentDebugContext() llvm.DebugDescriptor {
	return c.debug_context[len(c.debug_context)-1]
}

func (c *compiler) setDebugLine(pos token.Pos) {
	file := c.fileset.File(pos)
	ld := &llvm.LineDescriptor{
		Line:    uint32(file.Line(pos)),
		Context: c.currentDebugContext(),
	}
	c.builder.SetCurrentDebugLocation(c.debug_info.MDNode(ld))
}

// Debug intrinsic collectors.
func createGlobalVariableMetadata(global llvm.Value) llvm.DebugDescriptor {
	return &llvm.GlobalVariableDescriptor{
		Name:        global.Name(),
		DisplayName: global.Name(),
		//File:
		//Line:
		Type:  int32_debug_type, // FIXME
		Value: global}
}

// Creates a new named variable allocated on the stack.
// If value is not nil, its contents is stored in the allocated stack space.
func (c *compiler) newStackVar(argument int, stackf *LLVMValue, v types.Object, value llvm.Value, name string) (stackvalue llvm.Value, stackvar *LLVMValue) {
	typ := v.Type()

	// We need to put alloca instructions in the top block or the values
	// displayed when inspecting these variables in a debugger will be
	// completely messed up.
	curBlock := c.builder.GetInsertBlock()
	if p := curBlock.Parent(); !p.IsNil() {
		fb := p.FirstBasicBlock()
		fi := fb.FirstInstruction()
		if !fb.IsNil() && !fi.IsNil() {
			c.builder.SetInsertPointBefore(fi)
		}
	}
	old := c.builder.CurrentDebugLocation()
	c.builder.SetCurrentDebugLocation(llvm.Value{})
	stackvalue = c.builder.CreateAlloca(c.types.ToLLVM(typ), name)

	// For arguments we want to insert the store instruction
	// without debug information to ensure that they are executed
	// (and hence have proper values) before the debugger hits the
	// first line in a function.
	if argument == 0 {
		c.builder.SetCurrentDebugLocation(old)
		c.builder.SetInsertPointAtEnd(curBlock)
	}

	if !value.IsNil() {
		c.builder.CreateStore(value, stackvalue)
	}
	c.builder.SetCurrentDebugLocation(old)
	c.builder.SetInsertPointAtEnd(curBlock)

	ptrvalue := c.NewValue(stackvalue, types.NewPointer(typ))
	stackvar = ptrvalue.makePointee()
	stackvar.stack = stackf
	c.objectdata[v].Value = stackvar

	file := c.fileset.File(v.Pos())
	tag := llvm.DW_TAG_auto_variable
	if argument > 0 {
		tag = llvm.DW_TAG_arg_variable
	}
	ld := llvm.NewLocalVariableDescriptor(tag)
	ld.Argument = uint32(argument)
	ld.Line = uint32(file.Line(v.Pos()))
	ld.Name = name
	ld.File = &llvm.ContextDescriptor{llvm.FileDescriptor(file.Name())}
	ld.Type = c.tollvmDebugDescriptor(typ)
	ld.Context = c.currentDebugContext()
	c.builder.InsertDeclare(c.module.Module, llvm.MDNode([]llvm.Value{stackvalue}), c.debug_info.MDNode(ld))
	return stackvalue, stackvar
}

var uniqueId uint32

func (c *compiler) createBlockMetadata(stmt *ast.BlockStmt) llvm.DebugDescriptor {
	uniqueId++
	file := c.fileset.File(stmt.Pos())
	fd := llvm.FileDescriptor(file.Name())
	return &llvm.BlockDescriptor{
		File:    &fd,
		Line:    uint32(file.Line(stmt.Pos())),
		Context: c.currentDebugContext(),
		Id:      uniqueId,
	}
}

func (c *compiler) createFunctionMetadata(f *ast.FuncDecl, fn *LLVMValue) llvm.DebugDescriptor {
	file := c.fileset.File(f.Pos())
	fnptr := fn.value
	fun := fnptr.IsAFunction()
	if fun.IsNil() {
		fnptr = llvm.ConstExtractValue(fn.value, []uint32{0})
	}
	meta := &llvm.SubprogramDescriptor{
		Name:        fnptr.Name(),
		DisplayName: f.Name.Name,
		Path:        llvm.FileDescriptor(file.Name()),
		Line:        uint32(file.Line(f.Pos())),
		ScopeLine:   uint32(file.Line(f.Body.Pos())),
		Context:     &llvm.ContextDescriptor{llvm.FileDescriptor(file.Name())},
		Function:    fnptr}

	var result types.Type
	var metaparams []llvm.DebugDescriptor
	if ftyp, ok := fn.Type().(*types.Signature); ok {
		if recv := ftyp.Recv(); recv != nil {
			metaparams = append(metaparams, c.tollvmDebugDescriptor(recv.Type()))
		}
		if ftyp.Params() != nil {
			for i := 0; i < ftyp.Params().Len(); i++ {
				p := ftyp.Params().At(i)
				metaparams = append(metaparams, c.tollvmDebugDescriptor(p.Type()))
			}
		}
		if ftyp.Results() != nil {
			result = ftyp.Results().At(0).Type()
			// TODO: what to do with multiple returns?
			for i := 1; i < ftyp.Results().Len(); i++ {
				p := ftyp.Results().At(i)
				metaparams = append(metaparams, c.tollvmDebugDescriptor(p.Type()))
			}
		}
	}

	meta.Type = llvm.NewSubroutineCompositeType(c.tollvmDebugDescriptor(result), metaparams)
	c.compile_unit.Subprograms = append(c.compile_unit.Subprograms, meta)
	return meta
}

func (c *compiler) createMetadata() {
	functions := []llvm.DebugDescriptor{}
	globals := []llvm.DebugDescriptor{}
	debugInfo := &llvm.DebugInfo{}

	// Create global metadata.
	//
	// TODO We should store the global llgo.Value objects, and refer to them,
	// so we can calculate the type metadata properly.
	global := c.module.FirstGlobal()
	for ; !global.IsNil(); global = llvm.NextGlobal(global) {
		if !global.IsAGlobalVariable().IsNil() {
			name := global.Name()
			if ast.IsExported(name) {
				descriptor := createGlobalVariableMetadata(global)
				globals = append(globals, descriptor)
				c.module.AddNamedMetadataOperand(
					"llvm.dbg.gv", debugInfo.MDNode(descriptor))
			}
		}
	}

	compile_unit := &llvm.CompileUnitDescriptor{
		Language: llvm.DW_LANG_Go,
		//Path:
		Producer:        LLGOProducer,
		Runtime:         LLGORuntimeVersion,
		Subprograms:     functions,
		GlobalVariables: globals}

	// TODO resolve descriptors.
	c.module.AddNamedMetadataOperand(
		"llvm.dbg.cu", debugInfo.MDNode(compile_unit))
}
