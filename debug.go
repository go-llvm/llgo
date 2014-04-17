// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"debug/dwarf"
	"go/token"

	"code.google.com/p/go.tools/go/ssa"
	"code.google.com/p/go.tools/go/types"

	"github.com/go-llvm/llgo/debug"
	"github.com/go-llvm/llvm"
)

const (
	// non-standard debug metadata tags
	tagAutoVariable dwarf.Tag = 0x100
	tagArgVariable  dwarf.Tag = 0x101
)

type debugInfo struct {
	debug.DebugInfo
	debug.TypeMap
	module       llvm.Module
	blockUid     uint32
	cu           map[*token.File]*debug.CompileUnitDescriptor
	debugContext []debug.DebugDescriptor
}

func (d *debugInfo) pushContext(dd debug.DebugDescriptor) {
	d.debugContext = append(d.debugContext, dd)
}

func (d *debugInfo) popContext() (top debug.DebugDescriptor) {
	n := len(d.debugContext)
	top, d.debugContext = d.debugContext[n-1], d.debugContext[:n-1]
	return top
}

func (d *debugInfo) context() debug.DebugDescriptor {
	if len(d.debugContext) == 0 {
		return nil
	}
	return d.debugContext[len(d.debugContext)-1]
}

func (d *debugInfo) pushBlockContext(pos token.Pos) {
	uid := d.blockUid
	d.blockUid++
	file := d.Fset.File(pos)
	d.pushContext(&debug.BlockDescriptor{
		File:    &d.getCompileUnit(file).Path,
		Line:    uint32(file.Line(pos)),
		Context: d.context(),
		Id:      uid,
	})
}

func (d *debugInfo) popBlockContext() {
	d.popContext()
}

func (d *debugInfo) getCompileUnit(file *token.File) *debug.CompileUnitDescriptor {
	if d.cu == nil {
		d.cu = make(map[*token.File]*debug.CompileUnitDescriptor)
	}
	cu := d.cu[file]
	if cu == nil {
		var path string
		if file != nil {
			path = d.Fset.File(file.Pos(0)).Name()
		}
		cu = &debug.CompileUnitDescriptor{
			Language: debug.DW_LANG_Go,
			Path:     debug.FileDescriptor(path),
			Producer: "llgo",
			Runtime:  LLGORuntimeVersion,
		}
		d.cu[file] = cu
	}
	return cu
}

func (d *debugInfo) pushFunctionContext(fnptr llvm.Value, sig *types.Signature, pos token.Pos) {
	subprog := &debug.SubprogramDescriptor{
		Name:        fnptr.Name(),
		DisplayName: fnptr.Name(),
		Function:    fnptr,
	}
	file := d.Fset.File(pos)
	cu := d.getCompileUnit(file)
	subprog.File = file.Name()
	subprog.Context = &cu.Path
	if file != nil {
		subprog.Line = uint32(file.Line(pos))
		subprog.ScopeLine = uint32(file.Line(pos)) // TODO(axw)
	}
	sigType := d.TypeDebugDescriptor(sig).(*debug.CompositeTypeDescriptor)
	subroutineType := sigType.Members[0]
	subprog.Type = subroutineType
	cu.Subprograms = append(cu.Subprograms, subprog)
	d.pushContext(subprog)
}

func (d *debugInfo) popFunctionContext() {
	d.popContext()
}

// declare creates an llvm.dbg.declare call for the specified function
// parameter or local variable.
func (d *debugInfo) declare(b llvm.Builder, v ssa.Value, llv llvm.Value, paramIndex int) {
	tag := tagAutoVariable
	if paramIndex >= 0 {
		tag = tagArgVariable
	}
	ld := debug.NewLocalVariableDescriptor(tag)
	ld.Argument = uint32(paramIndex + 1)
	ld.Name = llv.Name()
	if file := d.Fset.File(v.Pos()); file != nil {
		ld.Line = uint32(file.Position(v.Pos()).Line)
		ld.File = &d.getCompileUnit(file).Path
	}
	ld.Type = d.TypeDebugDescriptor(deref(v.Type()))
	ld.Context = d.context()
	b.InsertDeclare(d.module, llvm.MDNode([]llvm.Value{llv}), d.MDNode(ld))
}

// value creates an llvm.dbg.value call for the specified register value.
func (d *debugInfo) value(b llvm.Builder, v ssa.Value, llv llvm.Value, paramIndex int) {
	// TODO(axw)
}

func (d *debugInfo) setLocation(b llvm.Builder, pos token.Pos) {
	position := d.Fset.Position(pos)
	b.SetCurrentDebugLocation(llvm.MDNode([]llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), uint64(position.Line), true),
		llvm.ConstInt(llvm.Int32Type(), uint64(position.Column), true),
		d.MDNode(d.context()),
		llvm.Value{},
	}))
}

// finalize must be called after all compilation units are translated,
// generating the final debug metadata for the module.
func (d *debugInfo) finalize() {
	for _, cu := range d.cu {
		d.module.AddNamedMetadataOperand("llvm.dbg.cu", d.MDNode(cu))
	}
	d.module.AddNamedMetadataOperand(
		"llvm.module.flags",
		llvm.MDNode([]llvm.Value{
			llvm.ConstInt(llvm.Int32Type(), 2, false), // Warn on mismatch
			llvm.MDString("Dwarf Version"),
			llvm.ConstInt(llvm.Int32Type(), 4, false),
		}),
	)
	d.module.AddNamedMetadataOperand(
		"llvm.module.flags",
		llvm.MDNode([]llvm.Value{
			llvm.ConstInt(llvm.Int32Type(), 1, false), // Error on mismatch
			llvm.MDString("Debug Info Version"),
			llvm.ConstInt(llvm.Int32Type(), 1, false),
		}),
	)
}
