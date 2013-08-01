; Copyright 2012 Andrew Wilkins.
; Use of this source code is governed by an MIT-style
; license that can be found in the LICENSE file.
;
; Defines low-level atomic operation APIs.

; FIXME(axw) align properly on 32-bit architectures.
; Maybe have separate files for 32-bit and 64-bit.

define void @"sync/atomic.StoreInt32"(i32*, i32) {
entry:
	store atomic i32 %1, i32* %0 seq_cst, align 4
	ret void
}

define void @"sync/atomic.StoreInt64"(i64*, i64) {
entry:
	store atomic i64 %1, i64* %0 seq_cst, align 8
	ret void
}

define i32 @"sync/atomic.LoadInt32"(i32*) {
entry:
	%1 = load atomic i32* %0 seq_cst, align 4
	ret i32 %1
}

define i64 @"sync/atomic.LoadInt64"(i64*) {
entry:
	%1 = load atomic i64* %0 seq_cst, align 8
	ret i64 %1
}

define i32 @"sync/atomic.AddInt32"(i32*, i32) {
entry:
	%old = atomicrmw add i32* %0, i32 %1 seq_cst
	%new = add i32 %old, %1
        ret i32 %new
}

define i64 @"sync/atomic.AddInt64"(i64*, i64) {
entry:
	%old = atomicrmw add i64* %0, i64 %1 seq_cst
	%new = add i64 %old, %1
        ret i64 %new
}

define i1 @"sync/atomic.CompareAndSwapInt32"(i32* %mem, i32 %cmp, i32 %new) {
	%old = cmpxchg i32* %mem, i32 %cmp, i32 %new seq_cst
	%success = icmp eq i32 %old, %cmp
	ret i1 %success
}

define i1 @"sync/atomic.CompareAndSwapInt64"(i64* %mem, i64 %cmp, i64 %new) {
	%old = cmpxchg i64* %mem, i64 %cmp, i64 %new seq_cst
	%success = icmp eq i64 %old, %cmp
	ret i1 %success
}

@"sync/atomic.StoreUint32" = alias void (i32*, i32)* @"sync/atomic.StoreInt32"
@"sync/atomic.StoreUint64" = alias void (i64*, i64)* @"sync/atomic.StoreInt64"
@"sync/atomic.LoadUint32" = alias i32 (i32*)* @"sync/atomic.LoadInt32"
@"sync/atomic.LoadUint64" = alias i64 (i64*)* @"sync/atomic.LoadInt64"
@"sync/atomic.AddUint32" = alias i32 (i32*, i32)* @"sync/atomic.AddInt32"
@"sync/atomic.AddUint64" = alias i64 (i64*, i64)* @"sync/atomic.AddInt64"
@"sync/atomic.CompareAndSwapUint32" = alias i1 (i32*, i32, i32)* @"sync/atomic.CompareAndSwapInt32"
@"sync/atomic.CompareAndSwapUint64" = alias i1 (i64*, i64, i64)* @"sync/atomic.CompareAndSwapInt64"

