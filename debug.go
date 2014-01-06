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
		Name:      t.String(),
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

func (c *compiler) popDebugContext() (top llvm.DebugDescriptor) {
	n := len(c.debug_context)
	top, c.debug_context = c.debug_context[n-1], c.debug_context[:n-1]
	return top
}

func (c *compiler) currentDebugContext() llvm.DebugDescriptor {
	if len(c.debug_context) == 0 {
		return nil
	}
	return c.debug_context[len(c.debug_context)-1]
}

func (c *compiler) setDebugLine(pos token.Pos) {
	ctx := c.currentDebugContext()
	if ctx == nil {
		return
	}
	file := c.fileset.File(pos)
	ld := &llvm.LineDescriptor{
		Line:    uint32(file.Line(pos)),
		Context: ctx,
	}
	c.builder.SetCurrentDebugLocation(c.debug_info.MDNode(ld))
}

var uniqueId uint32

func (c *compiler) createBlockMetadata(stmt *ast.BlockStmt) llvm.DebugDescriptor {
	ctx := c.currentDebugContext()
	if ctx == nil {
		return nil
	}
	uniqueId++
	file := c.fileset.File(stmt.Pos())
	fd := llvm.FileDescriptor(file.Name())
	return &llvm.BlockDescriptor{
		File:    &fd,
		Line:    uint32(file.Line(stmt.Pos())),
		Context: ctx,
		Id:      uniqueId,
	}
}

func (c *compiler) createFunctionMetadata(f *ast.FuncDecl, fn *LLVMValue) llvm.DebugDescriptor {
	if len(c.debug_context) == 0 {
		return nil
	}

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

	meta.Type = llvm.NewSubroutineCompositeType(
		c.tollvmDebugDescriptor(result),
		metaparams,
	)

	// compile unit is the first context object pushed
	compileUnit := c.debug_context[0].(*llvm.CompileUnitDescriptor)
	compileUnit.Subprograms = append(compileUnit.Subprograms, meta)
	return meta
}

// createLocalVariableMetadata creates and returns a debug descriptor for
// a local variable in a function.
//
// obj is the go/types object for the var.
// paramIndex is 0 for "auto" variables (non-parameter stack vars), and a
// 1-based index for parameter vars.
func (c *compiler) createLocalVariableMetadata(obj types.Object, paramIndex int) llvm.DebugDescriptor {
	ctx := c.currentDebugContext()
	if ctx == nil {
		return nil
	}
	tag := llvm.DW_TAG_auto_variable
	if paramIndex > 0 {
		tag = llvm.DW_TAG_arg_variable
	}
	position := c.fileset.Position(obj.Pos())
	ld := llvm.NewLocalVariableDescriptor(tag)
	ld.Argument = uint32(paramIndex)
	ld.Line = uint32(position.Line)
	ld.Name = obj.Name()
	ld.File = &llvm.ContextDescriptor{llvm.FileDescriptor(position.Filename)}
	ld.Type = c.tollvmDebugDescriptor(obj.Type())
	ld.Context = ctx
	return ld
}
