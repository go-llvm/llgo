; Copyright 2012 Andrew Wilkins.
; Use of this source code is governed by an MIT-style
; license that can be found in the LICENSE file.

declare i8 @llvm.ctlz.i8(i8, i1)

define i8 @runtime.ctlz8(i8) {
	%2 = call i8 (i8, i1)* @llvm.ctlz.i8(i8 %0, i1 0)
	ret i8 %2
}

