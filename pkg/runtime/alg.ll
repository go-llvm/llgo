; Copyright 2013 Andrew Wilkins.
; Use of this source code is governed by an MIT-style
; license that can be found in the LICENSE file.

define i1 @runtime.eqalg(i1 (i64, i8*, i8*)*, i64, i8*, i8*) {
	%5 = call i1 %0(i64 %1, i8* %2, i8* %3)
	ret i1 %5
}

