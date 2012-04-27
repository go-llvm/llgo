/*
Copyright (c) 2011 Andrew Wilkins <axwalk@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

#include <alloca.h>
#include <pthread.h>
#include <string.h>

struct call_args_t
{
    void (*indirect_fn)(void*);
    void *fn_arg;
    size_t fn_argsize;
    volatile int init : 1;
};

static void* call_gofunction(void *arg)
{
    struct call_args_t *call_args = (struct call_args_t*)arg;
    void(*indirect_fn)(void*) = call_args->indirect_fn;
    void *fn_arg = alloca(call_args->fn_argsize);
    memcpy(fn_arg, call_args->fn_arg, call_args->fn_argsize);
    call_args->init = 1;
    indirect_fn(fn_arg);
    return NULL;
}

void llgo_newgoroutine(void (*indirect_fn)(void*), void *arg, size_t argsize)
{
    pthread_t thread;
    pthread_attr_t attr;
    struct call_args_t call_args = {indirect_fn, arg, argsize, 0};
    pthread_attr_init(&attr);
    pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_DETACHED);
    pthread_create(&thread, &attr, &call_gofunction, &call_args);
    while (!call_args.init)
        /*Do-nothing*/;
}

