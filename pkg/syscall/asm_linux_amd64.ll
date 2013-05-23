; Copyright 2012 Andrew Wilkins.
; Use of this source code is governed by an MIT-style
; license that can be found in the LICENSE file.
;
; Defines low-level Syscall functions.

target datalayout = "e-p:64:64:64-S128-i1:8:8-i8:8:8-i16:16:16-i32:32:32-i64:64:64-f16:16:16-f32:32:32-f64:64:64-f128:128:128-v64:64:64-v128:128:128-a0:0:64-s0:64:64-f80:128:128-n8:16:32:64"
target triple = "x86_64-unknown-linux"

; r1, r2, errno
%syscallres = type {i64, i64, i64}

; TODO check if LLVM will optimise a call from RawSyscall
; to RawSyscall6 with constant zero arguments.
define %syscallres @syscall.RawSyscall(i64, i64, i64, i64) {
entry:
	%4 = call {i64, i64} asm sideeffect "syscall\0A", "={ax},={dx},{ax},{di},{si},{dx},{r10},{r8},{r9}"(i64 %0, i64 %1, i64 %2, i64 %3, i64 0, i64 0, i64 0) nounwind
	%5 = extractvalue {i64, i64} %4, 0
	%6 = extractvalue {i64, i64} %4, 1
	%7 = icmp ult i64 %5, -4095
	br i1 %7, label %ok, label %error
error:
	%8 = sub i64 0, %5
	%9 = insertvalue %syscallres undef, i64 -1, 0
	%10 = insertvalue %syscallres %9, i64 0, 1
	%11 = insertvalue %syscallres %10, i64 %8, 2
	ret %syscallres %11
ok:
	%12 = insertvalue %syscallres undef, i64 %5, 0
	%13 = insertvalue %syscallres %12, i64 %6, 1
	%14 = insertvalue %syscallres %13, i64 0, 2
	ret %syscallres %14
}

define %syscallres @syscall.RawSyscall6(i64, i64, i64, i64, i64, i64, i64) {
entry:
	%7 = call {i64, i64} asm sideeffect "syscall\0A", "={ax},={dx},{ax},{di},{si},{dx},{r10},{r8},{r9}"(i64 %0, i64 %1, i64 %2, i64 %3, i64 %4, i64 %5, i64 %6) nounwind
	%8 = extractvalue {i64, i64} %7, 0
	%9 = extractvalue {i64, i64} %7, 1
	%10 = icmp ult i64 %8, -4095
	br i1 %10, label %ok, label %error
error:
	%11 = sub i64 0, %8
	%12 = insertvalue %syscallres undef, i64 -1, 0
	%13 = insertvalue %syscallres %12, i64 0, 1
	%14 = insertvalue %syscallres %13, i64 %11, 2
	ret %syscallres %14
ok:
	%15 = insertvalue %syscallres undef, i64 %8, 0
	%16 = insertvalue %syscallres %15, i64 %9, 1
	%17 = insertvalue %syscallres %16, i64 0, 2
	ret %syscallres %17
}

; No tie-in with runtime yet, since there's no scheduler. Just alias it.
@syscall.Syscall = alias %syscallres (i64, i64, i64, i64)* @syscall.RawSyscall
@syscall.Syscall6 = alias %syscallres (i64, i64, i64, i64, i64, i64, i64)* @syscall.RawSyscall6

