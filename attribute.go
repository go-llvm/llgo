// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"fmt"
	"github.com/axw/gollvm/llvm"
	"go/ast"
	"strings"
)

const AttributeCommentPrefix = "#llgo "

// Attribute represents an attribute associated with a
// global variable or function.
type Attribute interface {
	Apply(Value)
}

// parseAttribute parses zero or more #llgo comment attributes associated with
// a global variable or function. The comment group provided will be processed
// one line at a time using parseAttribute.
func parseAttributes(doc *ast.CommentGroup) []Attribute {
	var attributes []Attribute
	if doc == nil {
		return attributes
	}
	for _, comment := range doc.List {
		text := comment.Text[2:]
		if strings.HasPrefix(comment.Text, "/*") {
			text = text[:len(text)-2]
		}
		attr := parseAttribute(strings.TrimSpace(text))
		if attr != nil {
			attributes = append(attributes, attr)
		}
	}
	return attributes
}

// parseAttribute parses a single #llgo comment attribute associated with
// a global variable or function. The string provided will be parsed
// if it begins with AttributeCommentPrefix, otherwise nil is returned.
func parseAttribute(line string) Attribute {
	if !strings.HasPrefix(line, AttributeCommentPrefix) {
		return nil
	}
	line = strings.TrimSpace(line[len(AttributeCommentPrefix):])
	colon := strings.IndexRune(line, ':')
	var key, value string
	if colon == -1 {
		key = line
	} else {
		key, value = line[:colon], line[colon+1:]
	}
	switch key {
	case "linkage":
		return parseLinkageAttribute(value)
	case "name":
		return nameAttribute(strings.TrimSpace(value))
	case "attr":
		return parseLLVMAttribute(strings.TrimSpace(value))
	case "thread_local":
		return tlsAttribute{}
	default:
		// FIXME decide what to do here. return error? log warning?
		panic("unknown attribute key: " + key)
	}
	return nil
}

type linkageAttribute llvm.Linkage

func (a linkageAttribute) Apply(v Value) {
	global := v.(*LLVMValue).pointer.value
	global.SetLinkage(llvm.Linkage(a))
}

func parseLinkageAttribute(value string) linkageAttribute {
	var result linkageAttribute
	value = strings.Replace(value, ",", " ", -1)
	for _, field := range strings.Fields(value) {
		switch strings.ToLower(field) {
		case "private":
			result |= linkageAttribute(llvm.PrivateLinkage)
		case "linker_private":
			result |= linkageAttribute(llvm.LinkerPrivateLinkage)
		case "linker_private_weak":
			result |= linkageAttribute(llvm.LinkerPrivateWeakLinkage)
		case "internal":
			result |= linkageAttribute(llvm.InternalLinkage)
		case "available_externally":
			result |= linkageAttribute(llvm.AvailableExternallyLinkage)
		case "linkonce":
			result |= linkageAttribute(llvm.LinkOnceAnyLinkage)
		case "common":
			result |= linkageAttribute(llvm.CommonLinkage)
		case "weak":
			result |= linkageAttribute(llvm.WeakAnyLinkage)
		case "appending":
			result |= linkageAttribute(llvm.AppendingLinkage)
		case "extern_weak":
			result |= linkageAttribute(llvm.ExternalWeakLinkage)
		case "linkonce_odr":
			result |= linkageAttribute(llvm.LinkOnceODRLinkage)
		case "weak_odr":
			result |= linkageAttribute(llvm.WeakODRLinkage)
		case "external":
			result |= linkageAttribute(llvm.ExternalLinkage)
		case "dllimport":
			result |= linkageAttribute(llvm.DLLImportLinkage)
		case "dllexport":
			result |= linkageAttribute(llvm.DLLExportLinkage)
		}
	}
	return result
}

type nameAttribute string

func (a nameAttribute) Apply(v Value) {
	if _, isfunc := v.Type().(*types.Signature); isfunc {
		fn := v.LLVMValue()
		fn = llvm.ConstExtractValue(fn, []uint32{0})
		name := string(a)
		curr := fn.GlobalParent().NamedFunction(name)
		if !curr.IsNil() && curr != fn {
			if curr.BasicBlocksCount() != 0 {
				panic(fmt.Errorf("Want to take the name %s from a function that has a body!", name))
			}
			curr.SetName(name + "_llgo_replaced")
			curr.ReplaceAllUsesWith(fn)
		}
		fn.SetName(name)
	} else {
		global := v.(*LLVMValue).pointer.value
		global.SetName(string(a))
	}
}

func parseLLVMAttribute(value string) llvmAttribute {
	var result llvmAttribute
	value = strings.Replace(value, ",", " ", -1)
	for _, field := range strings.Fields(value) {
		switch strings.ToLower(field) {
		case "noreturn":
			result |= llvmAttribute(llvm.NoReturnAttribute)
		case "nounwind":
			result |= llvmAttribute(llvm.NoUnwindAttribute)
		case "noinline":
			result |= llvmAttribute(llvm.NoInlineAttribute)
		case "alwaysinline":
			result |= llvmAttribute(llvm.AlwaysInlineAttribute)
		}
	}
	return result
}

type llvmAttribute llvm.Attribute

func (a llvmAttribute) Apply(v Value) {
	if _, isfunc := v.Type().(*types.Signature); isfunc {
		fn := v.LLVMValue()
		fn = llvm.ConstExtractValue(fn, []uint32{0})
		fn.AddFunctionAttr(llvm.Attribute(a))
	} else {
		v.LLVMValue().AddAttribute(llvm.Attribute(a))
	}
}

type tlsAttribute struct{}

func (tlsAttribute) Apply(v Value) {
	global := v.(*LLVMValue).pointer.value
	global.SetThreadLocal(true)
}
