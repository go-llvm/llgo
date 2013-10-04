; Copyright 2013 Andrew Wilkins.
; Use of this source code is governed by an MIT-style
; license that can be found in the LICENSE file.

declare void @llvm.trap() noreturn nounwind

define void @runtime.llvm_trap() {
entry:
	call void @llvm.trap()
	ret void
}
