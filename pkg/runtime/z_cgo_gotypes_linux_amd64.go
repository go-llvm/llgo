//

// +build linux
// +build amd64

// Created by cgo - DO NOT EDIT

package runtime

import "unsafe"

type _ unsafe.Pointer

type _Ctype___jmp_buf [8]_Ctype_long

type _Ctype___sigset_t _Ctype_struct___0

type _Ctype_int int32

type _Ctype_jmp_buf [1]_Ctype_struct___jmp_buf_tag

type _Ctype_long int64

type _Ctype_struct___0 struct {
//line :1
	__val [16]_Ctype_ulong
//line :1
}

type _Ctype_struct___jmp_buf_tag struct {
//line :1
	__jmpbuf	_Ctype___jmp_buf
//line :1
	__mask_was_saved	_Ctype_int
//line :1
	_	[4]byte
//line :1
	__saved_mask	_Ctype___sigset_t
//line :1
}

type _Ctype_ulong uint64

type _Ctype_void [0]byte

