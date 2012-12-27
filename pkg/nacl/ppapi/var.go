// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

import (
	"unsafe"
)

type Var struct {
	Type  VarType
	_     int32 // pad to 8-byte alignment
	Value VarValue
}

type VarType int32

const (
	VarTypeUndefined VarType = 0
	VarTypeNull      VarType = 1
	VarTypeBool      VarType = 2
	VarTypeInt32     VarType = 3
	// TODO etc.
)

type VarValue float64

func MakeVarInt32(value int32) Var {
	var v Var
	v.Type = VarTypeInt32
	*(*int32)(unsafe.Pointer(&v.Value)) = value
	return v
}

func MakeVarString(s string) Var {
	var cstr unsafe.Pointer
	n := uint32(len(s))
	if n > 0 {
		bytes := []byte(s)
		cstr = unsafe.Pointer(&bytes[0])
	}
	var v Var
	varIface := module.BrowserInterface("PPB_Var;1.1").(*ppbVar1_1)
	varIface.varFromUtf8(&v, cstr, n)
	return v
}

type ppbVar1_1 struct {
	addRef      func(Var)
	release     func(Var)
	varFromUtf8 func(v *Var, data unsafe.Pointer, len uint32)
	varToUtf8   func(v Var, len *uint32) unsafe.Pointer
}
