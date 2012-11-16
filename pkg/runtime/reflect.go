// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

// #llgo name: reflect.call
func reflect_call(fn, arg unsafe.Pointer, n uint32) {
	// TODO This is going to get messy. This code will
	// obviously need to know the calling convention,
	// which we currently don't specify, so as to have
	// LLVM generate the most efficient code it can.
	//
	// Apart from the obvious option of specifying a
	// calling convention (C, I suppose), there is
	// another somewhat horrible option: generate a
	// separate shim function with a specific calling
	// convention specifically for reflection.
}
