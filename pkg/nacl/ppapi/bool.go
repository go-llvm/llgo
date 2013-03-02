// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

// ppbool represents a boolean value, compatible with the PP_Bool type from the
// PPAPI C API.
type ppBool int32

func ppboolFromBool(b bool) ppBool {
	if b {
		return ppBool(1)
	}
	return ppBool(0)
}

func (b ppBool) toBool() bool {
	if b == 0 {
		return false
	}
	return true
}

var ppFalse = ppboolFromBool(false)
var ppTrue = ppboolFromBool(true)
