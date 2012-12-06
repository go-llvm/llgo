// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

import (
	"unsafe"
)

type cstring uintptr

// makecstring creates a null-terminated
// "C string" from a string's UTF-8 bytes.
func makecstring(s string) cstring {
	bytes := make([]byte, len(s)+1)
	copy(bytes, s)
	return cstring(unsafe.Pointer(&bytes[0]))
}

func (s cstring) String() string {
	// FIXME we should use C.GoString instead,
	// but cgo and llgo aren't talking right now.
	var bytes []byte
	ptr := unsafe.Pointer(s)
	for {
		b := (*byte)(ptr)
		if *b == 0 {
			break
		}
		bytes = append(bytes, *b)
		ptr = unsafe.Pointer(uintptr(ptr) + 1)
	}
	return string(bytes)
}

type module int32

// FIXME set calling convention of function to C.
//
// Actually, don't bother, as we'll have to move a
// lot of this into C/LLVM IR later anyway, when
// function types change.
type ppbGetInterface func(cstring) unsafe.Pointer

// #llgo name: PPP_InitializeModule
func initializeModule(_ module, getBrowserInterface ppbGetInterface) int32 {
	// TODO load these (maybe only the less crucial ones) on demand.
	browserMessagingInterface = (*ppbMessaging1_0)(getBrowserInterface(makecstring("PPB_Messaging;1.0")))
	browserVarInterface = (*ppbVar1_1)(getBrowserInterface(makecstring("PPB_Var;1.1")))
	browserGraphics2DInterface = (*ppbGraphics2D1_0)(getBrowserInterface(makecstring("PPB_Graphics2D;1.0")))
	browserViewInterface = (*ppbView1_0)(getBrowserInterface(makecstring("PPB_View;1.0")))
	browserInstanceInterface = (*ppbInstance1_0)(getBrowserInterface(makecstring("PPB_Instance;1.0")))
	browserImageDataInterface = (*ppbImageData1_0)(getBrowserInterface(makecstring("PPB_ImageData;1.0")))
	return PP_OK
}

// #llgo name: PPP_ShutdownModule
func shutdownModule() {
	// We must define this to link with ppapi.
}

// #llgo name: PPP_GetInterface
func getInterface(name cstring) unsafe.Pointer {
	switch name.String() {
	case "PPP_Instance;1.1":
		return unsafe.Pointer(&exampleInstance)
	}
	return nil
}

// TODO store the ppbGetInterface from initializeModule,
// and load these interfaces on demand using sync.Once
var browserMessagingInterface *ppbMessaging1_0
var browserVarInterface *ppbVar1_1
var browserGraphics2DInterface *ppbGraphics2D1_0
var browserViewInterface *ppbView1_0
var browserInstanceInterface *ppbInstance1_0
var browserImageDataInterface *ppbImageData1_0
