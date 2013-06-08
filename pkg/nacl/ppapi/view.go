// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

type View Resource

func (v View) Rect() Rect {
	var r Rect
	viewIface := module.BrowserInterface("PPB_View;1.0").(*ppbView1_0)
	if viewIface.getRect(v, &r).toBool() {
		return r
	}
	return Rect{}
}

type ppbView1_0 struct {
	isView        func(Resource) ppBool
	getRect       func(View, *Rect) ppBool
	isFullscreen  func(View) ppBool
	isVisible     func(View) ppBool
	isPageVisible func(View) ppBool
	getClipRect   func(View, *Rect) ppBool
}
