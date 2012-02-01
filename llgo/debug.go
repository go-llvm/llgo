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
    "unicode"
    "utf8"
)

// LLVM constants.
const (
    LLVMDebugVersion = (11 << 16)
)

// DWARF constants.
const (
    DW_TAG_compile_unit = 0x11
    DW_TAG_variable = 0x34
    DW_TAG_base_type = 0x24

    // http://dwarfstd.org/ShowIssue.php?issue=101014.1&type=open
    DW_LANG_Go = 0x0016

    // Encoding attribute values
    DW_ATE_address = 0x01
    DW_ATE_boolean = 0x02
    DW_ATE_complex_float = 0x03
    DW_ATE_float = 0x04
    DW_ATE_signed = 0x05
    DW_ATE_signed_char = 0x06
    DW_ATE_unsigned = 0x07
    DW_ATE_unsigned_char = 0x08
    DW_ATE_imaginary_float = 0x09
    DW_ATE_packed_decimal = 0x0a
    DW_ATE_numeric_string = 0x0b
    DW_ATE_edited = 0x0c
    DW_ATE_signed_fixed = 0x0d
    DW_ATE_unsigned_fixed = 0x0e
    DW_ATE_decimal_float = 0x0f
    DW_ATE_UTF = 0x10
    DW_ATE_lo_user = 0x80
    DW_ATE_hi_user = 0xff
)

// llgo constants.
const (
    LLGOProducer = "llgo " + LLGOVersion +
                   " (Andrew Wilkins <axwalk@gmail.com>)"
)

// Go builtin types.
var (
    int32_debug_type = llvm.MDNode([]llvm.Value{
        llvm.ConstInt(
            llvm.Int32Type(), DW_TAG_base_type + LLVMDebugVersion, false),
        llvm.MDNode(nil),
        llvm.MDString("int32"),
        llvm.Value{nil},
        llvm.ConstInt(llvm.Int32Type(), 0, false),
        llvm.ConstInt(llvm.Int32Type(), 32, false),
        llvm.ConstInt(llvm.Int32Type(), 0, false),
        llvm.ConstInt(llvm.Int32Type(), 0, false),
        llvm.ConstInt(llvm.Int32Type(), DW_ATE_signed, false),
    })
)

// Debug intrinsic collectors.
func createGlobalVariableMetadata(global llvm.Value) llvm.Value {
    return llvm.MDNode([]llvm.Value{
        llvm.ConstInt(
            llvm.Int32Type(), DW_TAG_variable + LLVMDebugVersion, false),
        llvm.ConstInt(llvm.Int32Type(), 0, false),
        llvm.MDNode(nil),
        llvm.MDString(global.Name()),
        llvm.MDString(global.Name()),
        llvm.MDString(global.Name()),
        llvm.Value{nil},
        llvm.ConstInt(llvm.Int32Type(), 0, false),
        int32_debug_type, // TODO
        llvm.ConstNull(llvm.Int1Type()),
        llvm.ConstAllOnes(llvm.Int1Type()),
    });
}

func (c *compiler) createCompileUnitMetadata() {
    enumtypes := []llvm.Value{};
    retainedtypes := []llvm.Value{};
    functions := []llvm.Value{};
    globals := []llvm.Value{};

    // Create global metadata.
    global := c.module.FirstGlobal()
    for ; !global.IsNil(); global = llvm.NextGlobal(global) {
        if !global.IsAGlobalVariable().IsNil() {
            name := global.Name()
            first, _ := utf8.DecodeRuneInString(name)
            if unicode.IsUpper(first) {
                metadata := createGlobalVariableMetadata(global)
                c.module.AddNamedMetadataOperand("llvm.dbg.gv", metadata)
                globals = append(globals, metadata)
            }
        }
    }

    c.module.AddNamedMetadataOperand("llvm.dbg.cu",
                                        llvm.MDNode([]llvm.Value{
        llvm.ConstInt(
            llvm.Int32Type(), DW_TAG_compile_unit+LLVMDebugVersion, false),
        llvm.ConstInt(llvm.Int32Type(), 0, false),
        llvm.ConstInt(llvm.Int32Type(), DW_LANG_Go, false),
        llvm.MDString("filename.todo"),
        llvm.MDString("filename.directory.todo"),
        llvm.MDString(LLGOProducer),
        llvm.ConstAllOnes(llvm.Int1Type()), // TODO main compile unit?
        llvm.ConstNull(llvm.Int1Type()), // Not optimised until link time.
        llvm.MDString(""), // Flags - correct type?
        llvm.ConstInt(llvm.Int32Type(), LLGORuntimeVersion, false),
        llvm.MDNode(enumtypes),
        llvm.MDNode(retainedtypes),
        llvm.MDNode(functions),
        llvm.MDNode(globals),
    }))
}

// vim: set ft=go :

