// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"github.com/axw/gollvm/llvm"
	"go/ast"
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
)

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
