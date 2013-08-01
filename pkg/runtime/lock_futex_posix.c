#include <sched.h>

void runtime_osyield() __asm__("runtime.osyield");

void runtime_osyield() {
    sched_yield();
}

