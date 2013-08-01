#include "types.h"
#include <linux/futex.h>
#include <stdio.h>
#include <unistd.h>
#include <sys/syscall.h>
#include <sys/time.h>

void futexsleep(uint32_t *addr,
                uint32_t val,
                int64_t ns) __asm__("runtime.futexsleep");

void futexwakeup(uint32_t *addr,
                 uint32_t cnt) __asm__("runtime.futexwakeup");

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
        secs = ns/1000000000LL;
        // Avoid overflow
        if(secs > 1LL<<30)
            secs = 1LL<<30;
        ts.tv_sec = secs;
        ts.tv_nsec = ns%1000000000LL;
        tsp = &ts;
    }

    // Some Linux kernels have a bug where futex of
    // FUTEX_WAIT returns an internal error code
    // as an errno.  Libpthread ignores the return value
    // here, and so can we: as it says a few lines up,
    // spurious wakeups are allowed.
    syscall(SYS_futex, addr, FUTEX_WAIT, val, tsp, NULL, 0);
}

// If any procs are sleeping on addr, wake up at most cnt.
void futexwakeup(uint32_t *addr, uint32_t cnt)
{
    int ret = syscall(SYS_futex, addr, FUTEX_WAKE, cnt, NULL, NULL, 0);
    if(ret >= 0)
        return;

    // I don't know that futex wakeup can return
    // EAGAIN or EINTR, but if it does, it would be
    // safe to loop and call futex again.
    printf("futexwakeup addr=%p returned %d\n", addr, ret);
    *(int32_t*)0x1006 = 0x1006;
}
