// +build pnacl

#include "types.h"
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/time.h>

// PNaCl IRT
#include <irt.h>

void futexsleep(uint32_t *addr,
                uint32_t val,
                int64_t ns) __asm__("runtime.futexsleep");

void futexwakeup(uint32_t *addr,
                 uint32_t cnt) __asm__("runtime.futexwakeup");

static struct nacl_irt_futex futex_api;
static pthread_once_t futex_api_once = PTHREAD_ONCE_INIT;
static void init_futex_api(void)
{
    size_t result = nacl_interface_query(NACL_IRT_FUTEX_v0_1,
                                         &futex_api,
                                         sizeof(futex_api));
    if (result == 0)
        abort();
}

// Atomically,
//  if(*addr == val) sleep
// Might be woken up spuriously; that's allowed.
// Don't sleep longer than ns; ns < 0 means forever.
void futexsleep(uint32_t *addr, uint32_t val, int64_t ns)
{
    struct timespec ts, *tsp;
    int64_t secs;

    if(ns < 0)
        tsp = (struct timespec*)0;
    else {
        struct timespec now;
        if (clock_gettime(CLOCK_REALTIME, &now) == -1)
            abort();
        secs = ns/1000000000LL;
        // Avoid overflow
        if(secs > 1LL<<30)
            secs = 1LL<<30;
        ts.tv_sec = secs;
        ts.tv_nsec = ns%1000000000LL;
        ts.tv_sec += now.tv_sec;
        ts.tv_nsec += now.tv_nsec;
        if (ts.tv_nsec > 1000000000LL)
        {
            ++ts.tv_sec;
            ts.tv_nsec %= 1000000000LL;
        }
        tsp = &ts;
    }

    // We can ignore the result: as it says a few lines up,
    // spurious wakeups are allowed.
    pthread_once(&futex_api_once, &init_futex_api);
    futex_api.futex_wait_abs((volatile int*)addr, val, tsp);
}

// If any procs are sleeping on addr, wake up at most cnt.
void futexwakeup(uint32_t *addr, uint32_t cnt)
{
    int ret;
    pthread_once(&futex_api_once, &init_futex_api);
    futex_api.futex_wake((volatile int*)addr, cnt, (int*)&cnt);
}
