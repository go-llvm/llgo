#include <stdlib.h>
#include <time.h>

#include "asm.h"
#include "inttypes.h"

struct llgo_time
{
    int64_t sec;
    int32_t nsec;
};

struct llgo_time time_now() LLGO_ASM_EXPORT("time.now");

struct llgo_time time_now()
{
    struct timespec ts = {0};
    struct llgo_time t;
    clock_gettime(CLOCK_REALTIME, &ts);
    t.sec = ts.tv_sec;
    t.nsec = ts.tv_nsec;
    return t;
}

