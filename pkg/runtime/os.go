// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

// #llgo name: os.Args
// #llgo linkage: common
var os_Args []string

// #llgo name: syscall.envs
// #llgo linkage: common
var syscall_envs []string

func setosargs(argc int32, argv_ **byte, envp_ **byte) {
	os_Args = make([]string, argc)
	argv := uintptr(unsafe.Pointer(argv_))
	for i := int32(0); i < argc; i++ {
		arg := *(**byte)(unsafe.Pointer(argv))
		arglen := c_strlen(arg)
		str := _string{arg, int(arglen)}
		os_Args[i] = *(*string)(unsafe.Pointer(&str))
		argv += unsafe.Sizeof(argv)
	}

	envp := uintptr(unsafe.Pointer(envp_))
	for {
		env := *(**byte)(unsafe.Pointer(envp))
		if env == nil {
			break
		}
		envlen := c_strlen(env)
		str_ := _string{env, int(envlen)}
		str := *(*string)(unsafe.Pointer(&str_))
		syscall_envs = append(syscall_envs, str)
		envp += unsafe.Sizeof(envp)
	}
}
