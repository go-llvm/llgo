; Copyright 2013 Andrew Wilkins.
; Use of this source code is governed by an MIT-style
; license that can be found in the LICENSE file.

define void @runtime.ccall(i8*) {
	%2 = bitcast i8* %0 to void ()*
	call void %2()
	ret void
}

