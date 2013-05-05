#include "panic.h"
#include <stdlib.h>
#include <string.h>

// typeinfo
extern void* _ZTI5Eface;

// thread-local
__thread struct Panic *tlspanic = NULL;

// extern functions
void* __cxa_allocate_exception(size_t);
void __cxa_throw(void *exc, void *typeinfo, void (*dest)(void*)) __attribute__((noreturn));
void* __cxa_begin_catch(void* exceptionObject);
void __cxa_end_catch(void);
void __cxa_rethrow(void);
int32_t typeid_for(void*) __asm__("llvm.eh.typeid.for");

// runtime functions
void panic(struct Eface error)
		__asm__("runtime.panic_") __attribute__((noreturn));
void recover(int32_t indirect, struct Eface *error)
		__asm__("runtime.recover") __attribute__((noinline));
void* before_defers(void *exc, int32_t id)
		__asm__("runtime.before_defers") __attribute__((noinline));
void after_defers(struct Panic*)
		__asm__("runtime.after_defers") __attribute__((noinline));

void panic(struct Eface error) {
	void *e = __cxa_allocate_exception(sizeof(struct Eface));
	memcpy(e, &error, sizeof(struct Eface));
	__cxa_throw(e, &_ZTI5Eface, NULL);
}

void recover(int32_t indirect, struct Eface *error) {
	// (valid) call stack:
	//     recover
	//     deferred function
	//     <deferred function wrapper>
	//     run_defers
	//     catch-site
	int depth = indirect ? 4 : 3;
	if (tlspanic && tlspanic->caught == runtime_caller_region(depth)) {
		struct Panic *p = tlspanic;
		tlspanic = p->next;
		memcpy(error, &p->value, sizeof(struct Eface));
		return;
	}
	memset(error, 0, sizeof(struct Eface));
}

static void *non_go_exception = (void*)0xFFFFFFFF;

void* before_defers(void *exc, int32_t typeid) {
	if (typeid == typeid_for(&_ZTI5Eface)) {
		struct Eface *e = (struct Eface*)__cxa_begin_catch(exc);
		struct Panic *p = (struct Panic*)malloc(sizeof(struct Panic));
		p->next = tlspanic;
		p->caught = runtime_caller_region(1);
		p->recovered = 0;
		memcpy(&p->value, e, sizeof(struct Eface));
		tlspanic = p;
		return p;
	} else {
		return non_go_exception;
	}
	return NULL;
}

void after_defers(struct Panic *p) {
	if (p == non_go_exception) {
		__cxa_end_catch();
	} else if (p) {
		if (p->recovered) {
			free(p);
			__cxa_end_catch();
		} else if (p == tlspanic && p->caught == runtime_caller_region(1)) {
			tlspanic = p->next;
			free(p);
			__cxa_rethrow();
		}
	}
}

