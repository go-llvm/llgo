// Copyright 2014 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package debug

import (
	"fmt"
	"go/token"

	"code.google.com/p/go.tools/go/types"
	"code.google.com/p/go.tools/go/types/typeutil"
)

// TypeMap contains information for mapping Go types
// to LLVM debug descriptors.
type TypeMap struct {
	Sizes types.Sizes
	Fset  *token.FileSet
	m     typeutil.Map
}

var voidType = BasicTypeDescriptor{
	TypeDescriptorCommon: TypeDescriptorCommon{
		Name: "void",
	},
}

// TypeDebugDescriptor maps a Go type to an llvm.DebugDescriptor.
func (m *TypeMap) TypeDebugDescriptor(t types.Type) TypeDebugDescriptor {
	return m.typeDebugDescriptor(t, types.TypeString(nil, t))
}

func (m *TypeMap) typeDebugDescriptor(t types.Type, name string) TypeDebugDescriptor {
	// Signature needs to be handled specially, to preprocess
	// methods, moving the receiver to the parameter list.
	if t, ok := t.(*types.Signature); ok {
		return m.descriptorSignature(t, name)
	}
	if t == nil {
		return &voidType
	}
	if dt, ok := m.m.At(t).(TypeDebugDescriptor); ok {
		return dt
	}
	dt := m.descriptor(t, name)
	m.m.Set(t, dt)
	return dt
}

func (m *TypeMap) descriptor(t types.Type, name string) TypeDebugDescriptor {
	switch t := t.(type) {
	case *types.Basic:
		return m.descriptorBasic(t, name)
	case *types.Pointer:
		return m.descriptorPointer(t)
	case *types.Struct:
		return m.descriptorStruct(t, name)
	case *types.Named:
		return m.descriptorNamed(t)
	case *types.Array:
		return m.descriptorArray(t, name)
	case *types.Slice:
		return m.descriptorSlice(t, name)
	case *types.Map:
		return m.descriptorMap(t, name)
	case *types.Chan:
		return m.descriptorChan(t, name)
	case *types.Interface:
		return m.descriptorInterface(t, name)
	default:
		panic(fmt.Sprintf("unhandled type: %T", t))
	}
}

func (m *TypeMap) descriptorBasic(t *types.Basic, name string) TypeDebugDescriptor {
	switch t.Kind() {
	case types.String:
		return m.descriptorStruct(types.NewStruct([]*types.Var{
			types.NewVar(0, nil, "ptr", types.NewPointer(types.Typ[types.Uint8])),
			types.NewVar(0, nil, "len", types.Typ[types.Int]),
		}, nil), name)
	case types.UnsafePointer:
		return &BasicTypeDescriptor{
			TypeDescriptorCommon: TypeDescriptorCommon{
				Name:      name,
				Size:      uint64(m.Sizes.Sizeof(t) * 8),
				Alignment: uint64(m.Sizes.Alignof(t) * 8),
			},
			TypeEncoding: DW_ATE_unsigned,
		}
	default:
		bt := &BasicTypeDescriptor{
			TypeDescriptorCommon: TypeDescriptorCommon{
				Name:      t.String(),
				Size:      uint64(m.Sizes.Sizeof(t) * 8),
				Alignment: uint64(m.Sizes.Alignof(t) * 8),
			},
		}
		switch bi := t.Info(); {
		case bi&types.IsBoolean != 0:
			bt.TypeEncoding = DW_ATE_boolean
		case bi&types.IsUnsigned != 0:
			bt.TypeEncoding = DW_ATE_unsigned
		case bi&types.IsInteger != 0:
			bt.TypeEncoding = DW_ATE_signed
		case bi&types.IsFloat != 0:
			bt.TypeEncoding = DW_ATE_float
		case bi&types.IsComplex != 0:
			bt.TypeEncoding = DW_ATE_imaginary_float
		case bi&types.IsUnsigned != 0:
			bt.TypeEncoding = DW_ATE_unsigned
		default:
			panic(fmt.Sprintf("unhandled: %#v", t))
		}
		return bt
	}
}

func (m *TypeMap) descriptorPointer(t *types.Pointer) TypeDebugDescriptor {
	return NewPointerDerivedType(m.TypeDebugDescriptor(t.Elem()))
}

func (m *TypeMap) descriptorStruct(t *types.Struct, name string) TypeDebugDescriptor {
	members := make([]DebugDescriptor, t.NumFields())
	for i := range members {
		f := t.Field(i)
		member := NewMemberDerivedType(m.TypeDebugDescriptor(f.Type()))
		member.Name = f.Name()
		members[i] = member
	}
	dt := NewStructCompositeType(members)
	dt.Name = name
	return dt
}

func (m *TypeMap) descriptorNamed(t *types.Named) TypeDebugDescriptor {
	placeholder := &PlaceholderTypeDescriptor{}
	m.m.Set(t, placeholder)
	underlying := t.Underlying()
	if old, ok := m.m.At(underlying).(DebugDescriptor); ok {
		// Recreate the underlying type, in lieu of a method of cloning.
		m.m.Delete(underlying)
		defer m.m.Set(underlying, old)
	}
	dt := m.typeDebugDescriptor(underlying, t.String())
	if file := m.Fset.File(t.Obj().Pos()); file != nil {
		dt.Common().File = file.Name()
	}
	placeholder.TypeDebugDescriptor = dt
	return dt
}

func (m *TypeMap) descriptorArray(t *types.Array, name string) TypeDebugDescriptor {
	return NewArrayCompositeType(m.TypeDebugDescriptor(t.Elem()), t.Len())
}

func (m *TypeMap) descriptorSlice(t *types.Slice, name string) TypeDebugDescriptor {
	sliceStruct := types.NewStruct([]*types.Var{
		types.NewVar(0, nil, "ptr", types.NewPointer(t.Elem())),
		types.NewVar(0, nil, "len", types.Typ[types.Int]),
		types.NewVar(0, nil, "cap", types.Typ[types.Int]),
	}, nil)
	return m.typeDebugDescriptor(sliceStruct, name)
}

func (m *TypeMap) descriptorMap(t *types.Map, name string) TypeDebugDescriptor {
	return m.descriptorBasic(types.Typ[types.Uintptr], name)
}

func (m *TypeMap) descriptorChan(t *types.Chan, name string) TypeDebugDescriptor {
	return m.descriptorBasic(types.Typ[types.Uintptr], name)
}

func (m *TypeMap) descriptorInterface(t *types.Interface, name string) TypeDebugDescriptor {
	ifaceStruct := types.NewStruct([]*types.Var{
		types.NewVar(0, nil, "type", types.NewPointer(types.Typ[types.Uint8])),
		types.NewVar(0, nil, "data", types.NewPointer(types.Typ[types.Uint8])),
	}, nil)
	return m.typeDebugDescriptor(ifaceStruct, name)
}

func (m *TypeMap) descriptorSignature(t *types.Signature, name string) TypeDebugDescriptor {
	// If there's a receiver change the receiver to an
	// additional (first) parameter, and take the value of
	// the resulting signature instead.
	if recv := t.Recv(); recv != nil {
		params := t.Params()
		paramvars := make([]*types.Var, int(params.Len()+1))
		paramvars[0] = recv
		for i := 0; i < int(params.Len()); i++ {
			paramvars[i+1] = params.At(i)
		}
		params = types.NewTuple(paramvars...)
		t := types.NewSignature(nil, nil, params, t.Results(), t.Variadic())
		return m.typeDebugDescriptor(t, name)
	}
	if dt, ok := m.m.At(t).(TypeDebugDescriptor); ok {
		return dt
	}

	var returnType DebugDescriptor
	var paramTypes []DebugDescriptor
	if results := t.Results(); results.Len() == 1 {
		returnType = m.TypeDebugDescriptor(results.At(0).Type())
	} else if results != nil {
		fields := make([]DebugDescriptor, results.Len())
		for i := range fields {
			fields[i] = m.TypeDebugDescriptor(results.At(i).Type())
		}
		returnType = NewStructCompositeType(fields)
	}
	if params := t.Params(); params != nil && params.Len() > 0 {
		paramTypes = make([]DebugDescriptor, params.Len())
		for i := range paramTypes {
			paramTypes[i] = m.TypeDebugDescriptor(params.At(i).Type())
		}
	}
	ct := NewStructCompositeType([]DebugDescriptor{
		NewSubroutineCompositeType(returnType, paramTypes),
		m.TypeDebugDescriptor(types.NewPointer(types.Typ[types.Uint8])),
	})
	ct.Name = name
	m.m.Set(t, ct)
	return ct
}
