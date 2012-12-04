// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

type Instance int32

type pppInstance1_1 struct {
	didCreate          func(i Instance, argc uint32, argn, argv *cstring) ppbool
	didDestroy         func(i Instance)
	didChangeView      func(i Instance, view Resource)
	didChangeFocus     func(i Instance, hasFocus ppbool)
	handleDocumentLoad func(i Instance, urlLoader Resource) ppbool
}
