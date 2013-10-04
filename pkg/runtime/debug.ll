; Copyright 2013 The llgo Authors.
; Use of this source code is governed by an MIT-style
; license that can be found in the LICENSE file.

declare void @llvm.debugtrap() noreturn nounwind

define void @runtime.Breakpoint() {
entry:
	call void @llvm.debugtrap()
	ret void
}
