// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

type View Resource

func (v View) Rect() Rect {
	var r Rect
	if browserViewInterface.getRect(v, &r).toBool() {
		return r
	}
	return Rect{}
}

type ppbView1_0 struct {
	isView        func(Resource) ppbool
	getRect       func(View, *Rect) ppbool
	isFullscreen  func(View) ppbool
	isVisible     func(View) ppbool
	isPageVisible func(View) ppbool
	getClipRect   func(View, *Rect) ppbool
}
