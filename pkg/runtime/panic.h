#ifndef _LLGO_PANIC_H
#define _LLGO_PANIC_H

#include <setjmp.h>

#include "types.h"
#include "asm.h"

struct Defer {
	// f represents the deferred function.
	struct Func f;

	// next points to the next deferred function in the chain.
	struct Defer *next;
};

struct Defers {
	jmp_buf j;

	// caller identifies the function which generated
	// the deferred function; the value is obtained from
	// _Unwind_GetRegionStart.
	uintptr_t caller;

	struct Defer *d;

	struct Defers *next;
};

struct Eface {
	void *type;
	void *data;
};

struct Panic {
	struct Panic *next;
	struct Eface value;
};

// current_panic returns the panic stack
// for the calling thread.
struct Panic* current_panic()
	LLGO_ASM_EXPORT("runtime.current_panic");

// runtime_caller_region returns the instruction
// region of the call frame specified by the number
// of frames to skip from the current location.
uintptr_t runtime_caller_region(int skip)
	    LLGO_ASM_EXPORT("runtime.caller_region") __attribute__((noinline));

// guardedcall0 calls the given niladic function,
// preventing any panics from escaping.
void guardedcall0(struct Func f)
	LLGO_ASM_EXPORT("runtime.guardedcall0");

// guardedcall1 calls the given niladic function,
// preventing any panics from escaping, calling
// errback if one does occur.
void guardedcall1(struct Func f, struct Func errback)
	LLGO_ASM_EXPORT("runtime.guardedcall1");

#endif

