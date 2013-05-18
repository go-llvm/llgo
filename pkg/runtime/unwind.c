#include "panic.h"

#include <unwind.h>

/* Not declared by clang's unwind.h */
uintptr_t _Unwind_GetRegionStart(struct _Unwind_Context * context);

struct caller_region_arg {
	int       skip;
	uintptr_t result;
};

static _Unwind_Reason_Code
backtrace_caller_region(struct _Unwind_Context *ctx, void *arg_) {
	struct caller_region_arg *arg = (struct caller_region_arg*)arg_;
	if (arg->skip)
	{
		--arg->skip;
		return _URC_NO_REASON; // keep going
	}
	arg->result = _Unwind_GetRegionStart(ctx);
	return _URC_NORMAL_STOP; // break
}

/* caller_region gets the region start for the caller specified
 * by skip, where skip has the same meaning as in runtime.Caller. */
uintptr_t runtime_caller_region(int skip) {
	struct caller_region_arg arg;
	if (skip < 0)
		return 0;
	arg.skip = skip + 1; // +1 for this function
	_Unwind_Backtrace(&backtrace_caller_region, &arg);
	return arg.result;
}

