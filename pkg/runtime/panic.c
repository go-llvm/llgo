// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

#include "panic.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

// typeinfo
extern void* _ZTI5Eface;

// thread-local
__thread struct Panic *tlspanic = NULL;
__thread struct Defer *tlsdefer = NULL;

// extern functions
void* __cxa_allocate_exception(size_t);
void __cxa_throw(void *exc, void *typeinfo, void (*dest)(void*)) __attribute__((noreturn));

// runtime functions
void panic(struct Eface error)
		__asm__("runtime.panic_") __attribute__((noreturn));
void recover(int32_t indirect, struct Eface *error)
	__asm__("runtime.recover") __attribute__((noinline));
void pushdefer(struct Func)
	__asm__("runtime.pushdefer") __attribute__((noinline));
void rundefers(void)
	__asm__("runtime.rundefers") __attribute__((noinline));

void panic(struct Eface error) {
	struct Panic *p = (struct Panic*)malloc(sizeof(struct Panic));
	p->next = tlspanic;
	memcpy(&p->value, &error, sizeof(struct Eface));
	tlspanic = p;
	__cxa_throw(__cxa_allocate_exception(0), &_ZTI5Eface, NULL);
}

struct Panic* current_panic() {
    return tlspanic;
}

void recover(int32_t indirect, struct Eface *error) {
	// (valid) call stack:
	//     recover
	//     deferred function
	//     <deferred function wrapper>
	//     callniladic
	//     guardedcall0
	//     run_defers
	//     catch-site
	int depth = 5 + (indirect ? 1 : 0);
	if (tlspanic && tlsdefer && tlsdefer->caller == runtime_caller_region(depth)) {
		struct Panic *p = tlspanic;
		memcpy(error, &p->value, sizeof(struct Eface));
	    while (tlspanic) {
	        p = tlspanic->next;
	        free(tlspanic);
	        tlspanic = p;
	    }
		return;
	}
	memset(error, 0, sizeof(struct Eface));
}

void pushdefer(struct Func f) {
	struct Defer *d = (struct Defer*)malloc(sizeof(struct Defer));
	d->f = f;
	d->caller = runtime_caller_region(1);
	d->next = tlsdefer;
	tlsdefer = d;
}

void rundefers(void) {
	// FIXME cater for recursive calls.
	const uintptr_t caller = runtime_caller_region(1);
	for (; tlsdefer && tlsdefer->caller == caller;) {
	    struct Defer *d = tlsdefer;
	    guardedcall0(d->f);
	    tlsdefer = tlsdefer->next;
	    free(d);
	}
	if (tlspanic) {
		__cxa_throw(__cxa_allocate_exception(0), &_ZTI5Eface, NULL);
	}
}

