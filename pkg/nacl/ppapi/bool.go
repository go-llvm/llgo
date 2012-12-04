// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

// ppbool represents a boolean value, compatible with the PP_Bool type from the
// PPAPI C API.
type ppbool int32

func ppboolFromBool(b bool) ppbool {
	if b {
		return ppbool(1)
	}
	return ppbool(0)
}

func (b ppbool) toBool() bool {
	if b == 0 {
		return false
	}
	return true
}
