#ifndef _LLGO_PANIC_H
#define _LLGO_PANIC_H

#include "types.h"

struct Defer {
    // f represents the deferred function.
    struct Func f;

	// caller identifies the function which generated
    // the deferred function; the value is obtained from
	// _Unwind_GetRegionStart.
    uintptr_t caller;

    // next points to the next deferred function in the chain.
    struct Defer *next;
};

struct Eface {
	void *data;
	void *type;
};

struct Panic {
	struct Panic *next;
	struct Eface value;
};

// current_panic returns the panic stack
// for the calling thread.
struct Panic* current_panic()
    __asm__("runtime.current_panic");

// runtime_caller_region returns the instruction
// region of the call frame specified by the number
// of frames to skip from the current location.
uintptr_t runtime_caller_region(int skip)
		__asm__("runtime.caller_region") __attribute__((noinline));

// guardedcall0 calls the given niladic function,
// preventing any panics from escaping.
void guardedcall0(struct Func f)
	__asm__("runtime.guardedcall0");

#endif

