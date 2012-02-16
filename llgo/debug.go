/*
Copyright (c) 2011, 2012 Andrew Wilkins <axwalk@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package llgo

import (
    "github.com/axw/gollvm/llvm"
    "go/ast"
)

// llgo constants.
const (
    LLGOAuthor = "Andrew Wilkins <axwalk@gmail.com>"
    LLGOProducer = "llgo " + LLGOVersion + " (" + LLGOAuthor + ")"
)

// Go builtin types.
var (
    int32_debug_type = &llvm.BasicTypeDescriptor{
        Name: "int32",
        Size: 32,
        Alignment: 32,
        TypeEncoding: llvm.DW_ATE_signed,
    }
)

// Debug intrinsic collectors.
func createGlobalVariableMetadata(global llvm.Value) llvm.DebugDescriptor {
    return &llvm.GlobalVariableDescriptor{
        Name: global.Name(),
        DisplayName: global.Name(),
        //File:
        //Line:
        Type: int32_debug_type, // FIXME
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

    compile_unit := &llvm.CompileUnitDescriptor {
        Language: llvm.DW_LANG_Go,
        //Path: 
        Producer: LLGOProducer,
        Runtime: LLGORuntimeVersion,
        Subprograms: functions,
        GlobalVariables: globals}

    // TODO resolve descriptors.
    c.module.AddNamedMetadataOperand(
        "llvm.dbg.cu", debugInfo.MDNode(compile_unit))
}

// vim: set ft=go :

