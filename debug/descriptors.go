// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package debug

import (
	"debug/dwarf"
	"fmt"
	"path"

	"github.com/go-llvm/llvm"
)

type DebugInfo struct {
	cache map[DebugDescriptor]llvm.Value
}

type DebugDescriptor interface {
	// Tag returns the DWARF tag for this descriptor.
	Tag() dwarf.Tag

	// mdNode creates an LLVM metadata node.
	mdNode(i *DebugInfo) llvm.Value
}

type TypeDebugDescriptor interface {
	DebugDescriptor
	Common() *TypeDescriptorCommon
}

///////////////////////////////////////////////////////////////////////////////
// Utility functions.

func constInt1(v bool) llvm.Value {
	if v {
		return llvm.ConstAllOnes(llvm.Int1Type())
	}
	return llvm.ConstNull(llvm.Int1Type())
}

func (info *DebugInfo) mdFileNode(d *FileDescriptor) llvm.Value {
	if d == nil {
		return llvm.Value{}
	}
	return info.MDNode(d)
}

func (info *DebugInfo) MDNode(d DebugDescriptor) llvm.Value {
	if d == nil {
		return llvm.Value{nil}
	}
	if info.cache == nil {
		info.cache = make(map[DebugDescriptor]llvm.Value)
	}
	value, exists := info.cache[d]
	if !exists {
		// Create a dummy value to stop recursion,
		// and then replace it with the real value.
		//
		// We need a unique string here, as metadata
		// nodes are deduplicated: use the address of
		// the DebugInfo + len(info.cache).
		dummy := llvm.MDString(fmt.Sprintf("llgo/debug.DebugInfo:%p:%d", info, len(info.cache)))
		info.cache[d] = dummy
		value = d.mdNode(info)
		info.cache[d] = value
		dummy.ReplaceAllUsesWith(value)
	}
	return value
}

func (info *DebugInfo) MDNodes(d []DebugDescriptor) []llvm.Value {
	if n := len(d); n > 0 {
		v := make([]llvm.Value, n)
		for i := 0; i < n; i++ {
			v[i] = info.MDNode(d[i])
		}
		return v
	}
	return nil
}

// TypeDescriptorCommon is a struct containing fields common to all
// type descriptors.
type TypeDescriptorCommon struct {
	Context   DebugDescriptor
	Name      string
	File      string
	Line      uint32
	Size      uint64 // Size in bits.
	Alignment uint64 // Alignment in bits.
	Offset    uint64 // Offset in bits
	Flags     uint32
}

func (t *TypeDescriptorCommon) Common() *TypeDescriptorCommon {
	return t
}

// PlaceholderTypeDescriptor is a placeholder for type
// descriptors, and is used to avoid infinite recursion.
type PlaceholderTypeDescriptor struct {
	TypeDebugDescriptor
}

///////////////////////////////////////////////////////////////////////////////
// Basic Types

type BasicTypeDescriptor struct {
	TypeDescriptorCommon
	TypeEncoding DwarfTypeEncoding
}

func (d *BasicTypeDescriptor) Tag() dwarf.Tag {
	return dwarf.TagBaseType
}

func (d *BasicTypeDescriptor) mdNode(info *DebugInfo) llvm.Value {
	return llvm.MDNode([]llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), llvm.LLVMDebugVersion+uint64(d.Tag()), false),
		FileDescriptor(d.File).path(),
		info.MDNode(d.Context),
		llvm.MDString(d.Name),
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Line), false),
		llvm.ConstInt(llvm.Int64Type(), d.Size, false),
		llvm.ConstInt(llvm.Int64Type(), d.Alignment, false),
		llvm.ConstInt(llvm.Int64Type(), d.Offset, false),
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Flags), false),
		llvm.ConstInt(llvm.Int32Type(), uint64(d.TypeEncoding), false),
	})
}

///////////////////////////////////////////////////////////////////////////////
// Composite Types

type CompositeTypeDescriptor struct {
	TypeDescriptorCommon
	tag     dwarf.Tag
	Base    DebugDescriptor // Type derived from.
	Members []DebugDescriptor
}

func (d *CompositeTypeDescriptor) Tag() dwarf.Tag {
	return d.tag
}

func (d *CompositeTypeDescriptor) mdNode(info *DebugInfo) llvm.Value {
	return llvm.MDNode([]llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), llvm.LLVMDebugVersion+uint64(d.Tag()), false),
		FileDescriptor(d.File).path(),
		info.MDNode(d.Context),
		llvm.MDString(d.Name),
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Line), false),
		llvm.ConstInt(llvm.Int64Type(), d.Size, false),
		llvm.ConstInt(llvm.Int64Type(), d.Alignment, false),
		llvm.ConstInt(llvm.Int64Type(), d.Offset, false),
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Flags), false),
		info.MDNode(d.Base), // reference type derived from
		llvm.MDNode(info.MDNodes(d.Members)),
		llvm.ConstInt(llvm.Int32Type(), uint64(0), false), // Runtime language
		info.MDNode(nil),                                  // Base type containing the vtable pointer for this type
		info.MDNode(nil),                                  // template parameters
		info.MDNode(nil),                                  // type UID
	})
}

func NewStructCompositeType(Members []DebugDescriptor) *CompositeTypeDescriptor {
	d := new(CompositeTypeDescriptor)
	d.tag = dwarf.TagStructType
	d.Members = Members // XXX Take a copy?
	return d
}

func NewArrayCompositeType(elem DebugDescriptor, n int64) *CompositeTypeDescriptor {
	return &CompositeTypeDescriptor{
		tag:     dwarf.TagArrayType,
		Base:    elem,
		Members: []DebugDescriptor{subrangeDescriptor{high: n - 1}},
	}
}

func NewSubroutineCompositeType(result DebugDescriptor, params []DebugDescriptor) *CompositeTypeDescriptor {
	return &CompositeTypeDescriptor{
		tag:     dwarf.TagSubroutineType,
		Members: append([]DebugDescriptor{result}, params...),
	}
}

///////////////////////////////////////////////////////////////////////////////
// Subrange

type subrangeDescriptor struct {
	low, high int64
}

func (d subrangeDescriptor) Tag() dwarf.Tag {
	return dwarf.TagSubrangeType
}

func (d subrangeDescriptor) mdNode(info *DebugInfo) llvm.Value {
	return llvm.MDNode([]llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Tag())+llvm.LLVMDebugVersion, false),
		llvm.ConstInt(llvm.Int64Type(), uint64(d.low), true),
		llvm.ConstInt(llvm.Int64Type(), uint64(d.high), true),
	})
}

///////////////////////////////////////////////////////////////////////////////
// Compilation Unit

type CompileUnitDescriptor struct {
	Path            FileDescriptor // Path to file being compiled.
	Language        DwarfLang
	Producer        string
	Optimized       bool
	CompilerFlags   string
	Runtime         int32
	EnumTypes       []DebugDescriptor
	RetainedTypes   []DebugDescriptor
	Subprograms     []DebugDescriptor
	GlobalVariables []DebugDescriptor
}

func (d *CompileUnitDescriptor) Tag() dwarf.Tag {
	return dwarf.TagCompileUnit
}

func mdNodeList(info *DebugInfo, list []DebugDescriptor) llvm.Value {
	if len(list) > 0 {
		return llvm.MDNode(info.MDNodes(list))
	}
	return llvm.MDNode([]llvm.Value{llvm.ConstNull(llvm.Int32Type())})
}

func (d *CompileUnitDescriptor) mdNode(info *DebugInfo) llvm.Value {
	return llvm.MDNode([]llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Tag())+llvm.LLVMDebugVersion, false),
		d.Path.path(),
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Language), false),
		llvm.MDString(d.Producer),
		constInt1(d.Optimized),
		llvm.MDString(d.CompilerFlags),
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Runtime), false),
		mdNodeList(info, d.EnumTypes),
		mdNodeList(info, d.RetainedTypes),
		mdNodeList(info, d.Subprograms),
		mdNodeList(info, d.GlobalVariables),
		mdNodeList(info, nil), // List of imported entities
		llvm.MDString(""),     // Split debug filename
	})
}

///////////////////////////////////////////////////////////////////////////////
// Derived Types

type DerivedTypeDescriptor struct {
	TypeDescriptorCommon
	tag  dwarf.Tag
	Base DebugDescriptor
}

func (d *DerivedTypeDescriptor) Tag() dwarf.Tag {
	return d.tag
}

func (d *DerivedTypeDescriptor) mdNode(info *DebugInfo) llvm.Value {
	return llvm.MDNode([]llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), llvm.LLVMDebugVersion+uint64(d.Tag()), false),
		FileDescriptor(d.File).path(),
		info.MDNode(d.Context),
		llvm.MDString(d.Name),
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Line), false),
		llvm.ConstInt(llvm.Int64Type(), d.Size, false),
		llvm.ConstInt(llvm.Int64Type(), d.Alignment, false),
		llvm.ConstInt(llvm.Int64Type(), d.Offset, false),
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Flags), false),
		info.MDNode(d.Base),
	})
}

func NewMemberDerivedType(base DebugDescriptor) *DerivedTypeDescriptor {
	return &DerivedTypeDescriptor{tag: dwarf.TagMember, Base: base}
}

func NewPointerDerivedType(base DebugDescriptor) *DerivedTypeDescriptor {
	return &DerivedTypeDescriptor{tag: dwarf.TagPointerType, Base: base}
}

func NewParamDerivedType(base DebugDescriptor) *DerivedTypeDescriptor {
	return &DerivedTypeDescriptor{tag: dwarf.TagFormalParameter, Base: base}
}

///////////////////////////////////////////////////////////////////////////////
// Subprograms.

type SubprogramDescriptor struct {
	File        string
	Context     DebugDescriptor
	Name        string
	DisplayName string
	Line        uint32
	Type        DebugDescriptor
	Function    llvm.Value
	ScopeLine   uint32
	// Function declaration descriptor
	// Function variables
}

func (d *SubprogramDescriptor) Tag() dwarf.Tag {
	return dwarf.TagSubprogram
}

func (d *SubprogramDescriptor) mdNode(info *DebugInfo) llvm.Value {
	return llvm.MDNode([]llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), llvm.LLVMDebugVersion+uint64(d.Tag()), false),
		FileDescriptor(d.File).path(),
		info.MDNode(d.Context),
		llvm.MDString(d.Name),
		llvm.MDString(d.DisplayName),
		llvm.MDString(""), // mips linkage name
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Line), false),
		info.MDNode(d.Type),
		llvm.ConstNull(llvm.Int1Type()),           // not static
		llvm.ConstAllOnes(llvm.Int1Type()),        // locally defined (not extern)
		llvm.ConstNull(llvm.Int32Type()),          // virtuality
		llvm.ConstNull(llvm.Int32Type()),          // index into a virtual function
		info.MDNode(nil),                          // basetype containing the vtable pointer
		llvm.ConstInt(llvm.Int32Type(), 0, false), // flags
		llvm.ConstNull(llvm.Int1Type()),           // not optimised
		d.Function,
		info.MDNode(nil),                                            // Template parameters
		info.MDNode(nil),                                            // function declaration descriptor
		mdNodeList(info, nil),                                       // function variables
		llvm.ConstInt(llvm.Int32Type(), uint64(d.ScopeLine), false), // Line number where the scope of the subprogram begins
	})
}

///////////////////////////////////////////////////////////////////////////////
// Global Variables.

type GlobalVariableDescriptor struct {
	Context     DebugDescriptor
	Name        string
	DisplayName string
	File        *FileDescriptor
	Line        uint32
	Type        DebugDescriptor
	Local       bool
	External    bool
	Value       llvm.Value
}

func (d *GlobalVariableDescriptor) Tag() dwarf.Tag {
	return dwarf.TagVariable
}

func (d *GlobalVariableDescriptor) mdNode(info *DebugInfo) llvm.Value {
	return llvm.MDNode([]llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Tag())+llvm.LLVMDebugVersion, false),
		llvm.ConstNull(llvm.Int32Type()),
		info.MDNode(d.Context),
		llvm.MDString(d.Name),
		llvm.MDString(d.DisplayName),
		llvm.MDNode(nil),
		info.mdFileNode(d.File),
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Line), false),
		info.MDNode(d.Type),
		constInt1(d.Local),
		constInt1(!d.External),
		d.Value,
		llvm.MDNode(nil), // static member declaration
	})
}

///////////////////////////////////////////////////////////////////////////////
// Local Variables.

type LocalVariableDescriptor struct {
	tag      dwarf.Tag
	Context  DebugDescriptor
	Name     string
	File     *FileDescriptor
	Line     uint32
	Argument uint32
	Type     DebugDescriptor
}

func (d *LocalVariableDescriptor) Tag() dwarf.Tag {
	return d.tag
}

func (d *LocalVariableDescriptor) mdNode(info *DebugInfo) llvm.Value {
	return llvm.MDNode([]llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Tag())+llvm.LLVMDebugVersion, false),
		info.MDNode(d.Context),
		llvm.MDString(d.Name),
		info.mdFileNode(d.File),
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Line)|(uint64(d.Argument)<<24), false),
		info.MDNode(d.Type),
		llvm.ConstNull(llvm.Int32Type()), // flags
		llvm.ConstNull(llvm.Int32Type()), // optional reference to inline location
	})
}

func NewLocalVariableDescriptor(tag dwarf.Tag) *LocalVariableDescriptor {
	return &LocalVariableDescriptor{tag: tag}
}

///////////////////////////////////////////////////////////////////////////////
// Files.

type FileDescriptor string

func (d FileDescriptor) Tag() dwarf.Tag {
	return dwarf.TagFileType
}

func (d FileDescriptor) path() llvm.Value {
	if d == "" {
		d = "<synthetic>"
	}
	dirname, filename := path.Split(string(d))
	if l := len(dirname); l > 0 && dirname[l-1] == '/' {
		dirname = dirname[:l-1]
	}
	return llvm.MDNode([]llvm.Value{llvm.MDString(filename), llvm.MDString(dirname)})
}

func (d FileDescriptor) mdNode(info *DebugInfo) llvm.Value {
	return llvm.MDNode([]llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), llvm.LLVMDebugVersion+uint64(d.Tag()), false),
		d.path(),
	})
}

///////////////////////////////////////////////////////////////////////////////
// Block.

type BlockDescriptor struct {
	File    *FileDescriptor
	Context DebugDescriptor
	Line    uint32
	Column  uint32
	Id      uint32
}

func (d *BlockDescriptor) Tag() dwarf.Tag {
	return dwarf.TagLexDwarfBlock
}

func (d *BlockDescriptor) mdNode(info *DebugInfo) llvm.Value {
	return llvm.MDNode([]llvm.Value{
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Tag())+llvm.LLVMDebugVersion, false),
		info.mdFileNode(d.File),
		info.MDNode(d.Context),
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Line), false),
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Column), false),
		llvm.ConstInt(llvm.Int32Type(), 0, false), // DWARF path discriminator
		llvm.ConstInt(llvm.Int32Type(), uint64(d.Id), false),
	})
}
