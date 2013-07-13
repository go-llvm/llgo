; Copyright 2013 Andrew Wilkins.
; Use of this source code is governed by an MIT-style
; license that can be found in the LICENSE file.

declare void @free(i8*)
declare void @llvm.memcpy.p0i8.p0i8.i64(i8* nocapture, i8* nocapture, i64, i32, i1) nounwind
declare void @llvm.memmove.p0i8.p0i8.i64(i8* nocapture, i8* nocapture, i64, i32, i1) nounwind
declare void @llvm.memset.p0i8.i64(i8* nocapture, i8, i64, i32, i1) nounwind

define void @runtime.free(i64) {
entry:
  %1 = inttoptr i64 %0 to i8*
  tail call void @free(i8* %1)
  ret void
}

define void @runtime.memcpy(i64, i64, i64) {
entry:
  %3 = inttoptr i64 %0 to i8*
  %4 = inttoptr i64 %1 to i8*
  call void @llvm.memcpy.p0i8.p0i8.i64(i8* %3, i8* %4, i64 %2, i32 1, i1 false)
  ret void
}

define void @runtime.memmove(i64, i64, i64) {
entry:
  %3 = inttoptr i64 %0 to i8*
  %4 = inttoptr i64 %1 to i8*
  call void @llvm.memmove.p0i8.p0i8.i64(i8* %3, i8* %4, i64 %2, i32 1, i1 false)
  ret void
}

define void @runtime.memset(i64, i8, i64) {
entry:
  %3 = inttoptr i64 %0 to i8*
  call void @llvm.memset.p0i8.i64(i8* %3, i8 %1, i64 %2, i32 1, i1 false)
  ret void
}
