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

package runtime

import "unsafe"

// This is a runtime-internal representation of runtimeType.
// runtimeType is always proceeded by commonType.
type type_ struct {
	ptr unsafe.Pointer
	typ unsafe.Pointer
	commonType
}

type commonType struct {
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
	ptrToThis *type_
}

type uncommonType struct {
	name    *string
	pkgPath *string
	methods []method
}

type method struct {
	name    *string
	pkgPath *string
	mtyp    *type_
	typ     *type_
	ifn     unsafe.Pointer
	tfn     unsafe.Pointer
}

type sliceType struct {
	commonType
	elem *type_
}

type mapType struct {
	commonType
	key  *type_
	elem *type_
}

type imethod struct {
	name    *string
	pkgPath *string
	typ     *type_
}

type interfaceType struct {
	commonType
	methods []imethod
}

type ptrType struct {
	commonType
	elem *type_
}

type arrayType struct {
	commonType
	elem  *type_
	slice *type_
	len   uintptr
}

type chanType struct {
	commonType
	elem *type_
	dir  uintptr
}

type funcType struct {
	commonType
	dotdotdot bool
	in        []*type_
	out       []*type_
}

type structField struct {
	name    *string
	pkgPath *string
	typ     *type_
	tag     *string
	offset  uintptr
}

type structType struct {
	commonType
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
func eqtyp(t1, t2 *type_) bool {
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
		case chanKind:
		case funcKind:
		case interfaceKind:
		case mapKind:
		case ptrKind:
			t1 := (*ptrType)(unsafe.Pointer(&t1.commonType))
			t2 := (*ptrType)(unsafe.Pointer(&t2.commonType))
			return eqtyp(t1.elem, t2.elem)
		case sliceKind:
		case structKind:
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
