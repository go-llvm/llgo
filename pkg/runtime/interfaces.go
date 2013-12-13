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

// convertI2I takes a target interface type, a source interface value, and
// attempts to convert the source to the target type, storing the result
// in the provided structure.
//
// FIXME cache type-to-interface conversions.
func convertI2I(typ_, from_, to_ uintptr) bool {
	dyntypptr := (**rtype)(unsafe.Pointer(from_))
	if dyntypptr == nil {
		return false
	}
	dyntyp := *dyntypptr
	if dyntyp.uncommonType != nil {
		targettyp := (*interfaceType)(unsafe.Pointer(typ_))
		if len(targettyp.methods) > len(dyntyp.methods) {
			return false
		}
		for i, tm := range targettyp.methods {
			// TODO speed this up by iterating through in lockstep.
			found := false
			for _, sm := range dyntyp.methods {
				if *sm.name == *tm.name {
					if eqtyp(sm.typ, tm.typ) {
						fnptraddr := to_ + unsafe.Sizeof(to_)*uintptr(2+i)
						fnptrslot := (*uintptr)(unsafe.Pointer(fnptraddr))
						*fnptrslot = uintptr(sm.ifn)
						found = true
					}
					break
				}
			}
			if !found {
				return false
			}
		}
		targetvalue := (*uintptr)(unsafe.Pointer(to_ + unsafe.Sizeof(to_)))
		targetdyntyp := (**rtype)(unsafe.Pointer(to_))
		*targetvalue = *(*uintptr)(unsafe.Pointer(from_ + unsafe.Sizeof(from_)))
		*targetdyntyp = dyntyp
		return true
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

func convertE2I(e eface, typ unsafe.Pointer) (result iface) {
	result.tab = new(itab)
	result.data = e.data
	result.tab.inter = (*interfaceType)(typ)
	result.tab.typ = (*rtype)(typ)
	return result
}

// #llgo name: reflect.ifaceE2I
func reflect_ifaceE2I(t *rtype, src interface{}, dst unsafe.Pointer) {
	// TODO
	println("TODO: reflect.ifaceE2I")
	llvm_trap()
}
