// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

// This is a runtime-internal representation of runtimeType.
type rtype struct {
	size       uintptr
	hash       uint32
	_          uint8
	align      uint8
	fieldAlign uint8
	kind       uint8
	alg        *uintptr
	gc         unsafe.Pointer
	string     *string
	*uncommonType
	ptrToThis *rtype
}

type uncommonType struct {
	name    *string
	pkgPath *string
	methods []method
}

type method struct {
	name    *string
	pkgPath *string
	mtyp    *rtype
	typ     *rtype
	ifn     unsafe.Pointer
	tfn     unsafe.Pointer
}

type sliceType struct {
	rtype
	elem *rtype
}

type mapType struct {
	rtype
	key  *rtype
	elem *rtype
}

type imethod struct {
	name    *string
	pkgPath *string
	typ     *rtype
}

type interfaceType struct {
	rtype
	methods []imethod
}

type ptrType struct {
	rtype
	elem *rtype
}

type arrayType struct {
	rtype
	elem  *rtype
	slice *rtype
	len   uintptr
}

type chanType struct {
	rtype
	elem *rtype
	dir  uintptr
}

type funcType struct {
	rtype
	dotdotdot bool
	in        []*rtype
	out       []*rtype
}

type structField struct {
	name    *string
	pkgPath *string
	typ     *rtype
	tag     *string
	offset  uintptr
}

type structType struct {
	rtype
	fields []structField
}

const (
	invalidKind uint8 = iota
	boolKind
	intKind
	int8Kind
	int16Kind
	int32Kind
	int64Kind
	uintKind
	uint8Kind
	uint16Kind
	uint32Kind
	uint64Kind
	uintptrKind
	float32Kind
	float64Kind
	complex64Kind
	complex128Kind
	arrayKind
	chanKind
	funcKind
	interfaceKind
	mapKind
	ptrKind
	sliceKind
	stringKind
	structKind
	unsafePointerKind
)

// eqtyp takes two runtime types and returns true
// iff they are equal.
func eqtyp(t1, t2 *rtype) bool {
	if t1 == t2 {
		return true
	} else if t1 == nil || t2 == nil {
		return false
	}
	if t1.kind == t2.kind {
		// TODO check rules for type equality.
		//
		// Named type equality is covered in the trivial
		// case, since there is only one definition for
		// each named type.
		//
		// Basic types are not checked for explicitly,
		// as we should never be comparing unnamed basic
		// types.
		switch t1.kind {
		case arrayKind:
			t1 := (*arrayType)(unsafe.Pointer(t1))
			t2 := (*arrayType)(unsafe.Pointer(t2))
			return t1.len == t2.len && eqtyp(t1.elem, t2.elem)
		case chanKind:
			t1 := (*chanType)(unsafe.Pointer(t1))
			t2 := (*chanType)(unsafe.Pointer(t2))
			return t1.dir == t2.dir && eqtyp(t1.elem, t2.elem)
		case funcKind:
			t1 := (*funcType)(unsafe.Pointer(t1))
			t2 := (*funcType)(unsafe.Pointer(t2))
			if t1.dotdotdot == t2.dotdotdot {
				nin, nout := len(t1.in), len(t1.out)
				if nin == len(t2.in) && nout == len(t2.out) {
					for i := 0; i < nin; i++ {
						if !eqtyp(t1.in[i], t2.in[i]) {
							return false
						}
					}
					for i := 0; i < nout; i++ {
						if !eqtyp(t1.out[i], t2.out[i]) {
							return false
						}
					}
					return true
				}
			}
		case interfaceKind:
			// TODO
		case mapKind:
			t1 := (*mapType)(unsafe.Pointer(t1))
			t2 := (*mapType)(unsafe.Pointer(t2))
			return eqtyp(t1.key, t2.key) && eqtyp(t1.elem, t2.elem)
		case ptrKind:
			t1 := (*ptrType)(unsafe.Pointer(t1))
			t2 := (*ptrType)(unsafe.Pointer(t2))
			return eqtyp(t1.elem, t2.elem)
		case sliceKind:
			t1 := (*sliceType)(unsafe.Pointer(t1))
			t2 := (*sliceType)(unsafe.Pointer(t2))
			return eqtyp(t1.elem, t2.elem)
		case structKind:
			// TODO
		}
	}
	return false
}

///////////////////////////////////////////////////////////////////////////////
// Types used in runtime function signatures.

type _string struct {
	str *uint8
	len int
}

type slice struct {
	array *uint8
	len   uint
	cap   uint
}

// interface{}
type eface struct {
	rtyp *rtype
	data *uint8
}

// interface{...}
type iface struct {
	tab  *itab
	data *uint8
}

type itab struct {
	inter  *interfaceType
	typ    *rtype
	link   *itab
	bad    int32
	unused int32
	fun    unsafe.Pointer
}
