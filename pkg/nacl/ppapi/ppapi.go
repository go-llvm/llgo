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

type Module int32

// FIXME set calling convention of function to C
type ppbGetInterface func(cstring) unsafe.Pointer

// #llgo name: PPP_InitializeModule
func initializeModule(m Module, getBrowserInterface ppbGetInterface) int32 {
	browserMessagingInterface = (*ppbMessaging1_0)(getBrowserInterface(makecstring("PPB_Messaging;1.0")))
	browserVarInterface = (*ppbVar1_1)(getBrowserInterface(makecstring("PPB_Var;1.1")))
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
		return unsafe.Pointer(&testInstance)
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////

var browserMessagingInterface *ppbMessaging1_0
var browserVarInterface *ppbVar1_1

func strToVar(s string) Var {
	if browserVarInterface != nil {
		var cstr cstring
		n := uint32(len(s))
		if n > 0 {
			bytes := []byte(s)
			cstr = cstring(unsafe.Pointer(&bytes[0]))
		}
		var v Var
		(*browserVarInterface).varFromUtf8(&v, cstr, n)
		return v
	}
	return makeUndefinedVar()
}

func testDidCreate(i Instance, argc uint32, argn, argv *cstring) ppbool {
	message := "alert:Hello from llgo!"
	if browserMessagingInterface != nil {
		v := strToVar(message)
		browserMessagingInterface.postMessage(i, v)
	}
	return ppboolFromBool(true)
}

func testDidDestroy(i Instance) {
}

func testDidChangeView(i Instance, view Resource) {
}

func testDidChangeFocus(i Instance, hasFocus ppbool) {
}

func testHandleDocumentLoad(i Instance, urlLoader Resource) ppbool {
	return ppboolFromBool(false)
}

var testInstance = pppInstance1_1{
	testDidCreate,
	testDidDestroy,
	testDidChangeView,
	testDidChangeFocus,
	testHandleDocumentLoad,
}
