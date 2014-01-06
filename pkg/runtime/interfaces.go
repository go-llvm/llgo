// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

func compareE2E(a, b eface) bool {
	typesSame := a.rtyp == b.rtyp
	if !typesSame {
		if a.rtyp == nil || b.rtyp == nil {
			// one, but not both nil
			return false
		}
	} else if a.rtyp == nil {
		// both nil
		return true
	}
	if eqtyp(a.rtyp, b.rtyp) {
		algs := unsafe.Pointer(a.rtyp.alg)
		eqPtr := unsafe.Pointer(uintptr(algs) + unsafe.Sizeof(algs))
		eqFn := *(*unsafe.Pointer)(eqPtr)
		var avalptr, bvalptr unsafe.Pointer
		if a.rtyp.size <= unsafe.Sizeof(a.data) {
			// value fits in pointer
			avalptr = unsafe.Pointer(&a.data)
			bvalptr = unsafe.Pointer(&b.data)
		} else {
			avalptr = unsafe.Pointer(a.data)
			bvalptr = unsafe.Pointer(b.data)
		}
		return eqalg(eqFn, a.rtyp.size, avalptr, bvalptr)
	}
	return false
}

// convertI2E takes a non-empty interface
// value and converts it to an empty one.
func convertI2E(i iface) eface {
	if i.tab == nil {
		return eface{nil, nil}
	}
	return eface{i.tab.typ, i.data}
}

const ptrsize = unsafe.Sizeof(uintptr(0))

func convertE2I(e eface, typ unsafe.Pointer) (ok bool, result iface) {
	if e.rtyp == nil {
		// nil conversion
		return true, iface{}
	}
	if e.rtyp.uncommonType == nil {
		// unnamed type, cannot succeed
		return false, iface{}
	}
	targetType := (*interfaceType)(unsafe.Pointer(typ))
	if len(targetType.methods) > len(e.rtyp.methods) {
		// too few methods
		return false, iface{}
	}
	size := unsafe.Sizeof(*result.tab)
	size += (uintptr(len(targetType.methods)) - 1) * ptrsize
	tab := (*itab)(malloc(size))
	for i, tm := range targetType.methods {
		found := false
		for _, sm := range e.rtyp.methods {
			if *sm.name == *tm.name {
				if eqtyp(sm.typ, tm.typ) {
					tabptr := unsafe.Pointer(tab)
					fnptraddr := uintptr(tabptr) + unsafe.Sizeof(*tab) - ptrsize + uintptr(i)*ptrsize
					*(*unsafe.Pointer)(unsafe.Pointer(fnptraddr)) = sm.ifn
					found = true
				}
				break
			}
		}
		if !found {
			free(unsafe.Pointer(tab))
			return false, iface{}
		}
	}
	result.tab = tab
	result.data = e.data
	result.tab.inter = targetType
	result.tab.typ = e.rtyp
	return true, result
}

func mustConvertE2I(e eface, typ unsafe.Pointer) (result iface) {
	ok, result := convertE2I(e, typ)
	if !ok {
		estring := "nil"
		if e.rtyp != nil {
			estring = *e.rtyp.string
		}
		ifaceType := (*interfaceType)(typ)
		panic("interface conversion: interface is " + estring + ", not " + *ifaceType.string)
	}
	return result
}

// #llgo name: reflect.ifaceE2I
func reflect_ifaceE2I(t *rtype, src interface{}, dst unsafe.Pointer) {
	// TODO
	println("TODO: reflect.ifaceE2I")
	llvm_trap()
}

func convertE2V(e eface, typ_, ptr unsafe.Pointer) bool {
	typ := (*rtype)(typ_)
	if !eqtyp(e.rtyp, typ) {
		return false
	}
	if typ.size == 0 {
		return true
	}
	var data unsafe.Pointer
	if typ.size <= unsafe.Sizeof(data) {
		data = unsafe.Pointer(&e.data)
	} else {
		data = unsafe.Pointer(e.data)
	}
	// TODO(axw) use copy alg
	memcpy(ptr, data, typ.size)
	return true
}

func mustConvertE2V(e eface, typ_, ptr unsafe.Pointer) {
	if !convertE2V(e, typ_, ptr) {
		estring := "nil"
		if e.rtyp != nil {
			estring = *e.rtyp.string
		}
		typ := (*rtype)(typ_)
		panic("interface conversion: interface is " + estring + ", not " + *typ.string)
	}
}
