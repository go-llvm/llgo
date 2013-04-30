; Copyright 2013 Andrew Wilkins.
; Use of this source code is governed by an MIT-style
; license that can be found in the LICENSE file.

%struct.Eface = type { i8*, i8* }

; Define a C++ std::typeinfo for struct.Eface (interface{})
@_ZTVN10__cxxabiv117__class_type_infoE = external global i8*
@_ZTS5Eface = linkonce_odr constant [7 x i8] c"5Eface\00"
@_ZTI5Eface = linkonce_odr unnamed_addr constant { i8*, i8* } { i8* bitcast (i8** getelementptr inbounds (i8** @_ZTVN10__cxxabiv117__class_type_infoE, i64 2) to i8*), i8* getelementptr inbounds ([7 x i8]* @_ZTS5Eface, i32 0, i32 0) }

declare i8* @__cxa_allocate_exception(i64)
declare void @__cxa_throw(i8*, i8*, i8*)
declare i8* @__cxa_begin_catch(i8*)
declare void @__cxa_end_catch()
declare void @__cxa_rethrow()
declare i32 @llvm.eh.typeid.for(i8*) nounwind readnone
;declare void @llvm.memcpy.p0i8.p0i8.i64(i8* nocapture, i8* nocapture, i64, i32, i1) nounwind

define void @runtime.panic_(%struct.Eface %error) noreturn {
	; Calculate the size of %struct.Eface.
	%sizeEfacePtr = getelementptr inbounds %struct.Eface* null, i32 1
	%sizeEface = ptrtoint %struct.Eface* %sizeEfacePtr to i64

	; Allocate space for the exception, store the interface{} and throw.
	%1 = call i8* @__cxa_allocate_exception(i64 %sizeEface) nounwind
	%2 = bitcast i8* %1 to %struct.Eface*
	store %struct.Eface %error, %struct.Eface* %2
	call void @__cxa_throw(i8* %1, i8* bitcast ({ i8*, i8* }* @_ZTI5Eface to i8*), i8* null) noreturn
	ret void
}

; This function is called after the landingpad, but before deferred functions
; are invoked. It is provided the result of the landingpad, and space into
; which the exception is stored.
define void @runtime.before_defers(i8* %exc, i32 %id, %struct.Eface* %err) {
	; XXX Currently doing as the LLVM programmer's guide says, and
	; checking the typeid to cater for inlining. This will only ever
	; matter if we inline, say, C++ code, though, so this can probably
	; disappear.
	%1 = call i32 @llvm.eh.typeid.for(i8* bitcast ({ i8*, i8* }* @_ZTI5Eface to i8*)) nounwind
	%2 = icmp eq i32 %id, %1
	br i1 %2, label %match, label %nonmatch
match:
	%3 = call i8* @__cxa_begin_catch(i8* %exc) nounwind
	%4 = bitcast i8* %3 to %struct.Eface*
	%5 = load %struct.Eface* %4
	store %struct.Eface %5, %struct.Eface* %err
	ret void
nonmatch:
	ret void
}

define void @runtime.after_defers(%struct.Eface %err) {
	; TODO use llvm.expect here to optimise for null case.
	%1 = extractvalue %struct.Eface %err, 0
	%2 = icmp ne i8* %1, null
	br i1 %2, label %isnotnull, label %isnull
isnotnull:
	call void @__cxa_rethrow() noreturn
	ret void
isnull:
	call void @__cxa_end_catch() noreturn
	ret void
}

