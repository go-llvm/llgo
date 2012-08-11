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

// This is based on commonType from src/pkg/reflect/type.go.
type commonType struct {
	size       uintptr
	hash       uint32
	_          uint8
	align      uint8
	fieldAlign uint8
	kind       uint8
	alg        *uintptr
	string     *string
	_          uintptr // *uncommonType
	_          uintptr // *runtimeType
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
