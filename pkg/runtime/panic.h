#ifndef _LLGO_PANIC_H
#define _LLGO_PANIC_H

#include "inttypes.h"

struct Func {
    void (*f)(void);
    void *data;
};

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

uintptr_t runtime_caller_region(int skip)
		__asm__("runtime.caller_region") __attribute__((noinline));

#endif

