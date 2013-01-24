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

func compareI2I(atyp_, btyp_, aval, bval uintptr) bool {
	atyp := (*rtype)(unsafe.Pointer(atyp_))
	btyp := (*rtype)(unsafe.Pointer(btyp_))
	if eqtyp(atyp, btyp) {
		algs := unsafe.Pointer(atyp.alg)
		eqPtr := unsafe.Pointer(uintptr(algs) + unsafe.Sizeof(algs))
		eqFn := *(*unsafe.Pointer)(eqPtr)
		var avalptr, bvalptr unsafe.Pointer
		if atyp.size <= unsafe.Sizeof(aval) {
			// value fits in pointer
			avalptr = unsafe.Pointer(&aval)
			bvalptr = unsafe.Pointer(&bval)
		} else {
			avalptr = unsafe.Pointer(aval)
			bvalptr = unsafe.Pointer(bval)
		}
		return eqalg(eqFn, atyp.size, avalptr, bvalptr)
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
			for j, sm := range dyntyp.methods {
				// TODO check method types are equal.
				if *sm.name == *tm.name {
					fnptraddr := to_ + unsafe.Sizeof(to_)*uintptr(2+i)
					fnptrslot := (*uintptr)(unsafe.Pointer(fnptraddr))
					*fnptrslot = uintptr(sm.ifn)
					found = true
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

// #llgo name: reflect.ifaceE2I
func reflect_ifaceE2I(t *rtype, src interface{}, dst unsafe.Pointer) {
	// TODO
	println("TODO: reflect.ifaceE2I")
	llvm_trap()
}
