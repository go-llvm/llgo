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

type equalFunc func(uintptr, *int8, *int8) bool

func compareI2I(atyp, btyp, aval, bval uintptr) bool {
	if atyp == btyp {
		atyp := (*commonType)(unsafe.Pointer(atyp))
		btyp := (*commonType)(unsafe.Pointer(btyp))
		algs := unsafe.Pointer(atyp.alg)
		eqPtr := unsafe.Pointer(uintptr(algs) + unsafe.Sizeof(*atyp.alg))
		eqFn := *(*equalFunc)(eqPtr)
		var avalptr, bvalptr *int8
		if atyp.size <= unsafe.Sizeof(aval) {
			// value fits in pointer
			avalptr = (*int8)(unsafe.Pointer(&aval))
			bvalptr = (*int8)(unsafe.Pointer(&bval))
		} else {
			avalptr = (*int8)(unsafe.Pointer(aval))
			bvalptr = (*int8)(unsafe.Pointer(bval))
		}
		return eqFn(atyp.size, avalptr, bvalptr)
	}
	return false
}
