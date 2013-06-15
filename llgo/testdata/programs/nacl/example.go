// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/axw/llgo/pkg/nacl/ppapi"
)

var module *ppapi.Module

func CreateModule() (*ppapi.Module, error) {
	println("CreateModule")
	var err error
	module, err = ppapi.NewModule(creator{})
	return module, err
}

type creator struct{}

func (_ creator) CreateInstance(i ppapi.PP_Instance) (ppapi.Instance, error) {
	println("CreateInstance")
	return &Example{i}, nil
}

type Example struct {
	ppapi.PP_Instance
}

func (x *Example) DidCreate(args map[string]string) error {
	println("DidCreate")
	module.PostMessage(x.PP_Instance, "alert:Hello from llgo!")
	return nil
}

func (x *Example) DidChangeView(v ppapi.View) {
	println("DidChangeView")
}

func (x *Example) DidChangeFocus(hasFocus bool) {
	println("DidChangeFocus")
}

/*
func exampleDidCreate(i PP_Instance, argc uint32, argn, argv *cstring) ppbool {
	// Uncomment the following to exercise the "PostMessage" API.
	// This is a simple way of sending messages to the browser.
	//
	// Take a look at "common.js" in the NaCl SDK examples to see
	// how to handle messages on the browser side.
	//
		message := "alert:Hello from llgo!"
		if browserMessagingInterface != nil {
			v := strToVar(message)
			browserMessagingInterface.postMessage(i, v)
		}
	//
	return ppboolFromBool(true)
}
*/

/*
func exampleDidChangeView(inst PP_Instance, view View) {
	rect := view.Rect()
	size := rect.Size
	gfx := browserGraphics2DInterface.create(inst, &size, ppboolFromBool(true))
	browserInstanceInterface.bindGraphics(inst, gfx)
	imageData := browserImageDataInterface.create(inst, ppIMAGEDATAFORMAT_RGBA_PREMUL, &size, ppboolFromBool(false))

	// Create a drawing state object which we'll
	// pass around with the completion callback.
	state := &drawingState{
		graphics:  gfx,
		imageData: imageData,
	}

	if !browserImageDataInterface.describe(imageData, &state.desc).toBool() {
		println("failed to describe image data")
		return
	}

	draw(state, 0)
}

// #llgo name: usleep
func usleep(uint) int32

type drawingState struct {
	graphics  Resource
	imageData Resource
	desc      ppImageDataDesc
	offset    int
}

func draw(data unsafe.Pointer, result int32) {
	state := (*drawingState)(data)
	n := state.offset
	r, g, b := byte(0), byte(0), byte(0)
	ptr := browserImageDataInterface.map_(state.imageData)
	for y := 0; y < state.desc.size.height; y++ {
		for x := 0; x < state.desc.size.width; x++ {
			offset := uintptr(x)*4 + uintptr(state.desc.stride)*uintptr(y)
			ptrrgba := (*[4]byte)(unsafe.Pointer(uintptr(ptr) + offset))
			ptrrgba[0] = r
			ptrrgba[1] = g
			ptrrgba[2] = b
		}
		switch n++; n {
		case 10:
			r, g, b = 255, 0, 0
		case 20:
			r, g, b = 0, 255, 0
		case 30:
			r, g, b = 0, 0, 255
		case 40:
			r, g, b = 0, 0, 0
			n = 0
		}
	}
	browserImageDataInterface.unmap(state.imageData)
	browserGraphics2DInterface.paintImageData(state.graphics, state.imageData, &Point{}, &Rect{Size: state.desc.size})
	callback := ppCompletionCallback{func_: draw, data: unsafe.Pointer(state)}
	state.offset = (state.offset + 1) % 40

	// Flush, calling back to this same function. The sleep is there
	// to rate limit the drawing, otherwise the CPU will be pinned.
	usleep(50000)
	browserGraphics2DInterface.flush(state.graphics, callback)
}
*/
