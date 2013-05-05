#ifndef _LLGO_PANIC_H
#define _LLGO_PANIC_H

#include "inttypes.h"

struct Eface {
	void *data;
	void *type;
};

struct Panic {
	struct Panic *next;
	struct Eface value;

	// caught identifies the function which most recently caught
	// the panic/exception; the value is obtained from
	// _Unwind_GetRegionStart.
	uintptr_t caught;

	// recovered is a boolean flag indicating whether or not
	// the panic was recovered.
	int recovered;
};

uintptr_t runtime_caller_region(int skip)
		__asm__("runtime.caller_region") __attribute__((noinline));

#endif

