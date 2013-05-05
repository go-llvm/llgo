; Copyright 2013 Andrew Wilkins.
; Use of this source code is governed by an MIT-style
; license that can be found in the LICENSE file.

; Define a C++ std::typeinfo for struct.Eface (interface{})
@_ZTVN10__cxxabiv117__class_type_infoE = external global i8*
@_ZTS5Eface = linkonce_odr constant [7 x i8] c"5Eface\00"
@_ZTI5Eface = linkonce_odr unnamed_addr constant { i8*, i8* } { i8* bitcast (i8** getelementptr inbounds (i8** @_ZTVN10__cxxabiv117__class_type_infoE, i64 2) to i8*), i8* getelementptr inbounds ([7 x i8]* @_ZTS5Eface, i32 0, i32 0) }

