; Copyright 2012 Andrew Wilkins.
; Use of this source code is governed by an MIT-style
; license that can be found in the LICENSE file.
;
; Defines low-level Syscall functions.

target datalayout = "e-p:32:32:32-S64-i1:8:8-i8:8:8-i16:16:16-i32:32:32-i64:64:64-f16:16:16-f32:32:32-f64:64:64-f128:128:128-v64:64:64-v128:64:128-a0:0:64-n32"
target triple = "armv7-none-linux-gnueabi"

; r1, r2, errno
%syscallres = type {i64, i64, i64}

; newoffset, err
%seekres = type {i64, i8*, i8*}

; TODO check if LLVM will optimise a call from RawSyscall
; to RawSyscall6 with constant zero arguments.
define %syscallres @syscall.RawSyscall(i64, i64, i64, i64) {
entry:
    ; TODO
	ret %syscallres undef
}

define %syscallres @syscall.RawSyscall6(i64, i64, i64, i64, i64, i64, i64) {
entry:
    ; TODO
	ret %syscallres undef
}

define %seekres @syscall.Seek(i32, i64, i32) {
    ; TODO
    ret %seekres undef
}

; No tie-in with runtime yet, since there's no scheduler. Just alias it.
@syscall.Syscall = alias %syscallres (i64, i64, i64, i64)* @syscall.RawSyscall
@syscall.Syscall6 = alias %syscallres (i64, i64, i64, i64, i64, i64, i64)* @syscall.RawSyscall6

