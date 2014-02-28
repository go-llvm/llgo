// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

#include "panic.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

// thread-local
__thread struct Panic *tlspanic = NULL;
__thread struct Defers *tlsdefers = NULL;

// runtime functions
void panic(struct Eface error)
	    LLGO_ASM_EXPORT("runtime.panic_") __attribute__((noreturn));
struct Eface recover(int32_t indirect)
	LLGO_ASM_EXPORT("runtime.recover_") __attribute__((noinline));
void pushdefer(struct Func)
	LLGO_ASM_EXPORT("runtime.pushdefer");
void initdefers(struct Defers *d)
	LLGO_ASM_EXPORT("runtime.initdefers") __attribute__((noinline));
void rundefers(void)
	LLGO_ASM_EXPORT("runtime.rundefers") __attribute__((noinline));
void callniladic(struct Func f)
	LLGO_ASM_EXPORT("runtime.callniladic") __attribute__((noinline));
void raise() __attribute__((noreturn));

// raise jumps to the next "defers" jmp_buf,
// aborting if there are none remaining.
void raise() {
	if (!tlsdefers) {
	    abort();
	}
	longjmp(tlsdefers->j, 1);
}

void panic(struct Eface error) {
	struct Panic *p = (struct Panic*)malloc(sizeof(struct Panic));
	p->next = tlspanic;
	memcpy(&p->value, &error, sizeof(struct Eface));
	tlspanic = p;
	raise();
}

static void pop_panic() {
	struct Panic *p = tlspanic->next;
	free(tlspanic);
	tlspanic = p;
}

struct Panic* current_panic() {
	return tlspanic;
}

struct Eface recover(int32_t indirect) {
	// (valid) call stack:
	//     recover
	//     deferred function
	//     <deferred function wrapper>
	//     callniladic
	//     guardedcall0
	//     run_defers
	//     catch-site
	struct Eface value;
	int depth = 5 + (indirect ? 1 : 0);
	int i;
	if (tlspanic && tlsdefers && tlsdefers->caller == runtime_caller_region(depth)) {
	    struct Panic *p = tlspanic;
	    struct Eface value = p->value;
	    while (tlspanic) {
	        p = tlspanic->next;
	        free(tlspanic);
	        tlspanic = p;
	    }
	    return value;
	} else {
	    value.type = value.data = (void*)0;
	    return value;
	}
}

void pushdefer(struct Func f) {
	struct Defer *d = (struct Defer*)malloc(sizeof(struct Defer));
	struct Defers *ds = tlsdefers;
	d->f = f;
	d->next = ds->d;
	ds->d = d;
}

void initdefers(struct Defers *d) {
	d->caller = runtime_caller_region(1);
	d->d = NULL;
	d->next = tlsdefers;
	tlsdefers = d;
}

void rundefers(void) {
	struct Defers *ds = tlsdefers;
	while (ds->d) {
	    struct Defer *d = ds->d;
	    guardedcall0(d->f);
	    ds->d = d->next;
	    free(d);
	}
	tlsdefers = ds->next;
	if (tlspanic) {
	    raise();
	}
}

void guardedcall0(struct Func f) {
	struct Defers defers;
	initdefers(&defers);
	if (setjmp(defers.j) == 0) {
		callniladic(f);
	} else {
		pop_panic();
	}
	rundefers();
}

void guardedcall1(struct Func f, struct Func errback) {
	struct Defers defers;
	initdefers(&defers);
	if (setjmp(defers.j) == 0) {
		callniladic(f);
	} else {
		callniladic(errback);
		pop_panic();
	}
	rundefers();
}
