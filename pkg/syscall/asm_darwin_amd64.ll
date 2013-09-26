; Copyright 2012 Andrew Wilkins.
; Use of this source code is governed by an MIT-style
; license that can be found in the LICENSE file.
;
; Defines low-level Syscall functions.
; Notes:
;	1.	The syscall convention differs from the "normal" call convention
;		and the registers used here are indeed intentional. See abi.pdf@A.2.1 list item 1.
;
;	2.	http://www.opensource.apple.com/source/xnu/xnu-792.13.8/osfmk/mach/i386/syscall_sw.h:
;		"For 64-bit users, the 32-bit syscall number is partitioned  with the high-order bits representing
;		 the class and low-order bits being the syscall number within that class."
;
;		So the added 2<<24 just means we are using the UNIX/BSD syscall class.
;
;	3.	The return value from the syscall differs from the linux kernel syscall abi defined in abi.pdf.
;		Instead of returning -errno, an error is indicated by setting the carry flag and rax contains the errno value.
;		I haven't found a good official reference document stating that this is how it works,
;		but it can be spotted in for example these sources:
;
;
;			http://www.opensource.apple.com/source/xnu/xnu-792.13.8/bsd/dev/i386/systemcalls.c
;			http://golang.org/src/pkg/syscall/asm_darwin_amd64.s
;			http://farid.hajji.name/blog/2010/02/05/return-values-of-freebsd-syscalls-in-assembly/
;

target datalayout = "e-p:64:64:64-S128-i1:8:8-i8:8:8-i16:16:16-i32:32:32-i64:64:64-f16:16:16-f32:32:32-f64:64:64-f128:128:128-v64:64:64-v128:128:128-a0:0:64-s0:64:64-f80:128:128-n8:16:32:64"
target triple = "x86_64-unknown-darwin"

; r1, r2, errno
%syscallres = type {i64, i64, i64}

; TODO check if LLVM will optimise a call from RawSyscall
; to RawSyscall6 with constant zero arguments.
define %syscallres @syscall.RawSyscall(i64, i64, i64, i64) {
entry:
	%unix_syscall_class = shl i64 2, 24
	%syscall_number = add i64 %unix_syscall_class, %0
	%4 = call {i64, i64} asm sideeffect "syscall\0A", "={ax},={dx},{ax},{di},{si},{dx},{r10},{r8},{r9}"(i64 %syscall_number, i64 %1, i64 %2, i64 %3, i64 0, i64 0, i64 0) nounwind
	%5 = extractvalue {i64, i64} %4, 0
	%6 = extractvalue {i64, i64} %4, 1
	%carry = call i1 asm "setc $0", "=r"()
	br i1 %carry, label %error, label %ok
error:
	%7 = insertvalue %syscallres undef, i64 -1, 0
	%8 = insertvalue %syscallres %7, i64 0, 1
	%9 = insertvalue %syscallres %8, i64 %5, 2
	ret %syscallres %9
ok:
	%10 = insertvalue %syscallres undef, i64 %5, 0
	%11 = insertvalue %syscallres %10, i64 %6, 1
	%12 = insertvalue %syscallres %11, i64 0, 2
	ret %syscallres %12
}

define %syscallres @syscall.RawSyscall6(i64, i64, i64, i64, i64, i64, i64) {
entry:
	%unix_syscall_class = shl i64 2, 24
	%syscall_number = add i64 %unix_syscall_class, %0
	%7 = call {i64, i64} asm sideeffect "syscall\0A", "={ax},={dx},{ax},{di},{si},{dx},{r10},{r8},{r9}"(i64 %syscall_number, i64 %1, i64 %2, i64 %3, i64 %4, i64 %5, i64 %6) nounwind
	%8 = extractvalue {i64, i64} %7, 0
	%9 = extractvalue {i64, i64} %7, 1
	%carry = call i1 asm "setc $0", "=r"()
	br i1 %carry, label %error, label %ok
error:
	%10 = insertvalue %syscallres undef, i64 -1, 0
	%11 = insertvalue %syscallres %10, i64 0, 1
	%12 = insertvalue %syscallres %11, i64 %8, 2
	ret %syscallres %12
ok:
	%13 = insertvalue %syscallres undef, i64 %8, 0
	%14 = insertvalue %syscallres %13, i64 %9, 1
	%15 = insertvalue %syscallres %14, i64 0, 2
	ret %syscallres %15
}

; No tie-in with runtime yet, since there's no scheduler. Just alias it.
@syscall.Syscall = alias %syscallres (i64, i64, i64, i64)* @syscall.RawSyscall
@syscall.Syscall6 = alias %syscallres (i64, i64, i64, i64, i64, i64, i64)* @syscall.RawSyscall6

