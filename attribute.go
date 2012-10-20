// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"github.com/axw/gollvm/llvm"
	"strings"
)

const AttributeCommentPrefix = "#llgo "

// Atribute represents an attribute associated with a
// global variable or function.
type Attribute interface {
	Apply(Value)
}

// parseAttribute parses #llgo comment attributes associated with
// a global variable or function. The string provided will be parsed
// if it begins with AttributeCommentPrefix, otherwise nil is returned.
func parseAttribute(line string) Attribute {
	if !strings.HasPrefix(line, AttributeCommentPrefix) {
		return nil
	}
	line = strings.TrimSpace(line[len(AttributeCommentPrefix):])
	colon := strings.IndexRune(line, ':')
	key, value := line[:colon], line[colon+1:]
	switch key {
	case "linkage":
		return parseLinkageAttribute(value)
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
			result |= llvm.PrivateLinkage
		case "linker_private":
			result |= llvm.LinkerPrivateLinkage
		case "linker_private_weak":
			result |= llvm.LinkerPrivateWeakLinkage
		case "internal":
			result |= llvm.InternalLinkage
		case "available_externally":
			result |= llvm.AvailableExternallyLinkage
		case "linkonce":
			result |= llvm.LinkOnceAnyLinkage
		case "common":
			result |= llvm.CommonLinkage
		case "weak":
			result |= llvm.WeakAnyLinkage
		case "appending":
			result |= llvm.AppendingLinkage
		case "extern_weak":
			result |= llvm.ExternalWeakLinkage
		case "linkonce_odr":
			result |= llvm.LinkOnceODRLinkage
		case "weak_odr":
			result |= llvm.WeakODRLinkage
		case "external":
			result |= llvm.ExternalLinkage
		case "dllimport":
			result |= llvm.DLLImportLinkage
		case "dllexport":
			result |= llvm.DLLExportLinkage
		}
	}
	return result
}
