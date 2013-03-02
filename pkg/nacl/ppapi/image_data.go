// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

import "unsafe"

type ppImageDataFormat int32

const (
	ppIMAGEDATAFORMAT_BGRA_PREMUL ppImageDataFormat = 0
	ppIMAGEDATAFORMAT_RGBA_PREMUL ppImageDataFormat = 1
)

type ppImageDataDesc struct {
	format ppImageDataFormat
	size   Size
	stride int32
}

type ppbImageData1_0 struct {
	getNativeImageDataFormat   func() ppImageDataFormat
	isImageDataFormatSupported func(ppImageDataFormat) ppBool
	create                     func(i PP_Instance, f ppImageDataFormat, s *Size, initToZero ppBool) Resource
	isImageData                func(Resource) ppBool
	describe                   func(Resource, *ppImageDataDesc) ppBool
	map_                       func(Resource) unsafe.Pointer
	unmap                      func(Resource)
}
