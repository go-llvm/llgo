// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

import (
	"errors"
	"unsafe"
)

var createInstanceRequiredError = errors.New("createInstance must not be nil")

// InstanceCreator is an interface used for creating
// module instances. An InstanceCreator must be passed
// to NewModule from the "main.CreateModule" function.
type InstanceCreator interface {
	// CreateInstance is invoked to instantiate the
	// module, for each <embed> tag in the web page.
	CreateInstance(PP_Instance) (Instance, error)
}

// ModuleInitialiser is an interface that may optionally
// be implemented by the value passed to NewModule.
type ModuleInitialiser interface {
	// InitModule is invoked to initialise a module.
	//
	// If the module requires any global initialisation
	// that involves calling into PPAPI, then it must
	// be done during or after the call to InitModule.
	InitModule() error
}

// Module represents a plugin module.
//
// Developers must implement a function with the signature
// "func CreateModule() (*Module, error)", in the main package.
type Module struct {
	// Core is an instance of the core interface for
	// doing basic global operations. This field is
	// guarantee to be non-nil during/after invocation
	// of init.
	Core

	instanceCreator     InstanceCreator
	getBrowserInterface ppbGetInterface
	instances           map[PP_Instance]Instance
}

// NewModule creates a new Module, initialising it with an InstanceCreator,
// which must be non-nil. The value passed in may optionally implement
// ModuleInitialiser, whose InitModule method will be invoked if implemented.
func NewModule(c InstanceCreator) (*Module, error) {
	if c == nil {
		return nil, createInstanceRequiredError
	}
	return &Module{instanceCreator: c}, nil
}

func (m *Module) BrowserInterface(name string) interface{} {
	result := callPPBGetInterface(m.getBrowserInterface, name)
	if result == nil {
		return nil
	}
	switch name {
	case "PPB_Var;1.1":
		return (*ppbVar1_1)(result)
	case "PPB_Messaging;1.0":
		return (*ppbMessaging1_0)(result)
	}
	panic("unimplemented")
}

func (m *Module) PluginInterface(name string) interface{} {
	panic("unimplemented")
}

func (m *Module) PostMessage(instance PP_Instance, value interface{}) error {
	messaging := m.BrowserInterface("PPB_Messaging;1.0").(*ppbMessaging1_0)
	v, err := MakeVar(value)
	if err != nil {
		return err
	}
	callPostMessage(messaging, instance, &v)
	// TODO release var
	return nil
}

///////////////////////////////////////////////////////////////////////////////

// #llgo name: ppapi_instanceDidCreate
func instanceDidCreate(i PP_Instance, argc uint32, argn_, argv_ *cstring) ppBool {
	//defer func() {
	//    // FIXME do something more appropriate with the error.
	//    if err := recover(); err != nil {
	//        println(err)
	//    }
	//}()
	inst, err := module.instanceCreator.CreateInstance(i)
	if err != nil {
		return ppFalse
	}
	args := make(map[string]string, argc)
	argn := uintptr(unsafe.Pointer(argn_))
	argv := uintptr(unsafe.Pointer(argv_))
	for i := uint32(0); i < argc; i++ {
		name := (*cstring)(unsafe.Pointer(argn))
		value := (*cstring)(unsafe.Pointer(argv))
		args[name.String()] = value.String()
		argn += unsafe.Sizeof(name)
		argv += unsafe.Sizeof(value)
	}
	err = inst.DidCreate(args)
	if err != nil {
		return ppFalse
	}
	module.instances[i] = inst
	return ppTrue
}

// #llgo name: ppapi_instanceDidDestroy
func instanceDidDestroy(i PP_Instance) {
	delete(module.instances, i)
}

// #llgo name: ppapi_instanceDidChangeView
func instanceDidChangeView(i PP_Instance, view View) {
	if inst, ok := module.instances[i]; ok {
		inst.DidChangeView(view)
	}
}

// #llgo name: ppapi_instanceDidChangeFocus
func instanceDidChangeFocus(i PP_Instance, hasFocus ppBool) {
	if inst, ok := module.instances[i]; ok {
		inst.DidChangeFocus(hasFocus.toBool())
	}
}

// #llgo name: ppapi_instanceHandleDocumentLoad
func instanceHandleDocumentLoad(i PP_Instance, urlLoader Resource) ppBool {
	// Ignored
	return ppFalse
}
