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

func convertE2I(e eface, typ unsafe.Pointer) (result iface) {
	if e.rtyp == nil || e.rtyp.uncommonType == nil {
		return iface{}
	}
	targetType := (*interfaceType)(unsafe.Pointer(typ))
	if len(targetType.methods) > len(e.rtyp.methods) {
		return iface{}
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
			return iface{}
		}
	}
	result.tab = tab
	result.data = e.data
	result.tab.inter = targetType
	result.tab.typ = e.rtyp
	return result
}

// #llgo name: reflect.ifaceE2I
func reflect_ifaceE2I(t *rtype, src interface{}, dst unsafe.Pointer) {
	// TODO
	println("TODO: reflect.ifaceE2I")
	llvm_trap()
}
