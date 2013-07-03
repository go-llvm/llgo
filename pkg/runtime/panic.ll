; Copyright 2013 Andrew Wilkins.
; Use of this source code is governed by an MIT-style
; license that can be found in the LICENSE file.

; Define a C++ std::typeinfo for struct.Eface (interface{})
@_ZTVN10__cxxabiv117__class_type_infoE = external global i8*
@_ZTS5Eface = linkonce_odr constant [7 x i8] c"5Eface\00"
@_ZTI5Eface = linkonce_odr unnamed_addr constant { i8*, i8* } { i8* bitcast (i8** getelementptr inbounds (i8** @_ZTVN10__cxxabiv117__class_type_infoE, i64 2) to i8*), i8* getelementptr inbounds ([7 x i8]* @_ZTS5Eface, i32 0, i32 0) }

%niladicfunc = type { void ()*, i8* }
declare void @runtime.callniladic(%niladicfunc)
declare i32 @__gxx_personality_v0(...)
declare i8* @__cxa_begin_catch(i8*)
declare void @__cxa_end_catch()

define void @runtime.guardedcall0(%niladicfunc) noinline {
entry:
    invoke void @runtime.callniladic(%niladicfunc %0)
        to label %end unwind label %unwind
unwind:
    %1 = landingpad { i8*, i32 } personality i32 (...)* @__gxx_personality_v0 catch i8* null
    %2 = extractvalue { i8*, i32 } %1, 0
    call i8* @__cxa_begin_catch(i8* %2)
    call void @__cxa_end_catch()
    br label %end
end:
    ret void
}

define void @runtime.guardedcall1(%niladicfunc, %niladicfunc) noinline {
entry:
    invoke void @runtime.callniladic(%niladicfunc %0)
        to label %end unwind label %unwind
unwind:
    %2 = landingpad { i8*, i32 } personality i32 (...)* @__gxx_personality_v0 catch i8* null
    %3 = extractvalue { i8*, i32 } %2, 0
    call i8* @__cxa_begin_catch(i8* %3)
    call void @runtime.callniladic(%niladicfunc %1)
    call void @__cxa_end_catch()
    br label %end
end:
    ret void
}

