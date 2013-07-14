#ifndef _LLGO_TYPES_H
#define _LLGO_TYPES_H

#include "inttypes.h"

struct Func {
    void (*f)(void);
    void *data;
};

#endif

