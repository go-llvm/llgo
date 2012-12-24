// Copyright 2012 Andrew Wilkins.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

// #llgo name: os.Args
// #llgo linkage: common
var os_Args []string

func setosargs(argc int, argv_ **byte) {
	os_Args = make([]string, argc)
	argv := uintptr(unsafe.Pointer(argv_))
	for i := 0; i < argc; i++ {
		arg := *(**byte)(unsafe.Pointer(argv))
		arglen := c_strlen(arg)
		str := _string{arg, arglen}
		os_Args[i] = *(*string)(unsafe.Pointer(&str))
		argv += unsafe.Sizeof(argv)
	}
}
