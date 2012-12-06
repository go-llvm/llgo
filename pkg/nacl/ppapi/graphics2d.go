// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

type ppbGraphics2D1_0 struct {
	create          func(i Instance, s *Size, alwaysOpaque ppbool) Resource
	isGraphics2D    func(Resource) ppbool
	describe        func(r Resource, s *Size, alwaysOpaque *ppbool) ppbool
	paintImageData  func(gfx, img Resource, topleft *Point, src *Rect)
	scroll          func(gfx Resource, clip *Rect, amount *Point)
	replaceContents func(gfx, img Resource)
	flush           func(gfx Resource, callback ppCompletionCallback) int32
}
