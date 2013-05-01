; Copyright 2013 Andrew Wilkins.
; Use of this source code is governed by an MIT-style
; license that can be found in the LICENSE file.

%struct.Eface = type { i8*, i8* }
@efacesize = unnamed_addr constant i64 ptrtoint (%struct.Eface* getelementptr inbounds (%struct.Eface* null, i32 1) to i64)

; Currently panicking error.
@perror = thread_local(initialexec) global %struct.Eface {i8* null, i8* null}

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
declare void @llvm.memcpy.p0i8.p0i8.i64(i8* nocapture, i8* nocapture, i64, i32, i1) nounwind
declare void @llvm.memset.p0i8.i64(i8* nocapture, i8, i64, i32, i1) nounwind

define void @runtime.panic_(%struct.Eface %error) noreturn {
	; Allocate space for the exception, store the interface{} and throw.
	%1 = load i64* @efacesize
	%2 = call i8* @__cxa_allocate_exception(i64 %1) nounwind
	%3 = bitcast i8* %2 to %struct.Eface*
	store %struct.Eface %error, %struct.Eface* %3
	call void @__cxa_throw(i8* %2, i8* bitcast ({ i8*, i8* }* @_ZTI5Eface to i8*), i8* null) noreturn
	ret void
}

define void @runtime.recover(%struct.Eface* %error) {
	%1 = bitcast %struct.Eface* @perror to i8*
	%2 = bitcast %struct.Eface* %error to i8*
	%3 = load i64* @efacesize
	call void @llvm.memcpy.p0i8.p0i8.i64(i8* %2, i8* %1, i64 %3, i32 0, i1 0)
	; Wipe the TLS value.
	call void @llvm.memset.p0i8.i64(i8* %1, i8 0, i64 %3, i32 0, i1 0)
	ret void
}

; This function is called after the landingpad, but before deferred functions
; are invoked. It is provided the result of the landingpad, and space into
; which the exception is stored.
define void @runtime.before_defers(i8* %exc, i32 %id) {
	; XXX Currently doing as the LLVM programmer's guide says, and
	; checking the typeid to cater for inlining. This will only ever
	; matter if we inline, say, C++ code, though, so this can probably
	; disappear.
	%1 = call i32 @llvm.eh.typeid.for(i8* bitcast ({ i8*, i8* }* @_ZTI5Eface to i8*)) nounwind
	%2 = icmp eq i32 %id, %1
	br i1 %2, label %match, label %nonmatch
match:
	%3 = call i8* @__cxa_begin_catch(i8* %exc) nounwind
	%4 = bitcast %struct.Eface* @perror to i8*
	%5 = load i64* @efacesize
	call void @llvm.memcpy.p0i8.p0i8.i64(i8* %4, i8* %3, i64 %5, i32 0, i1 0)
	ret void
nonmatch:
	ret void
}

define void @runtime.after_defers() {
	; TODO use llvm.expect here to optimise for null case.
	%1 = getelementptr %struct.Eface* @perror, i32 0, i32 0
	%2 = load i8** %1
	%3 = icmp ne i8* %2, null
	br i1 %3, label %isnotnull, label %isnull
isnotnull:
	call void @__cxa_rethrow() noreturn
	ret void
isnull:
	call void @__cxa_end_catch() noreturn
	ret void
}

