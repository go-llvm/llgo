// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

type ppbGraphics2D1_0 struct {
	create          func(i PP_Instance, s *Size, alwaysOpaque ppBool) Resource
	isGraphics2D    func(Resource) ppBool
	describe        func(r Resource, s *Size, alwaysOpaque *ppBool) ppBool
	paintImageData  func(gfx, img Resource, topleft *Point, src *Rect)
	scroll          func(gfx Resource, clip *Rect, amount *Point)
	replaceContents func(gfx, img Resource)
	flush           func(gfx Resource, callback ppCompletionCallback) int32
}
