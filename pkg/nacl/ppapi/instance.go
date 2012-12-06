// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

type Instance int32

type pppInstance1_1 struct {
	didCreate          func(i Instance, argc uint32, argn, argv *cstring) ppbool
	didDestroy         func(i Instance)
	didChangeView      func(i Instance, view View)
	didChangeFocus     func(i Instance, hasFocus ppbool)
	handleDocumentLoad func(i Instance, urlLoader Resource) ppbool
}

type ppbInstance1_0 struct {
	bindGraphics func(Instance, Resource) ppbool
	isFullFrame  func(Instance) ppbool
}
