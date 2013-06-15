// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

import (
	"fmt"
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

func MakeVar(value interface{}) (Var, error) {
	switch value := value.(type) {
	case int32:
		return MakeVarInt32(value), nil
	case string:
		return MakeVarString(value), nil
	}
	return Var{}, fmt.Errorf("Unhandled type %T", value)
}

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
	callVarFromUtf8(varIface, &v, cstr, n)
	return v
}

type ppbVar1_1 struct {
	addRef      uintptr //func(Var)
	release     uintptr //func(Var)
	varFromUtf8 uintptr //func(v *Var, data unsafe.Pointer, len uint32)
	varToUtf8   uintptr //func(v Var, len *uint32) unsafe.Pointer
}

// #llgo name: ppapi_callVarFromUtf8
func callVarFromUtf8(i *ppbVar1_1, v *Var, data unsafe.Pointer, n uint32)
