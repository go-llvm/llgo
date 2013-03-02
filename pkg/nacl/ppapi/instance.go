// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

type PP_Instance int32

type pppInstance1_1 struct {
	didCreate          func(i PP_Instance, argc uint32, argn, argv *cstring) ppBool
	didDestroy         func(i PP_Instance)
	didChangeView      func(i PP_Instance, view View)
	didChangeFocus     func(i PP_Instance, hasFocus ppBool)
	handleDocumentLoad func(i PP_Instance, urlLoader Resource) ppBool
}

type ppbInstance1_0 struct {
	bindGraphics func(PP_Instance, Resource) ppBool
	isFullFrame  func(PP_Instance) ppBool
}

type Instance interface {
	DidCreate(args map[string]string) error
	DidChangeView(View)
	DidChangeFocus(hasFocus bool)
}
