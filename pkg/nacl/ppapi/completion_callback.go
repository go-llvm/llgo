// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

import "unsafe"

type ppCompletionCallbackFlag int32

const (
	ppCOMPLETIONCALLBACK_FLAG_NONE     ppCompletionCallbackFlag = 0
	ppCOMPLETIONCALLBACK_FLAG_OPTIONAL ppCompletionCallbackFlag = 1 << iota
)

type ppCompletionCallback struct {
	func_ func(userData unsafe.Pointer, result int32)
	data  unsafe.Pointer
	flags ppCompletionCallbackFlag
}
