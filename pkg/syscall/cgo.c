// Copyright 2014 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

#include <errno.h>
#include <stdint.h>

uintptr_t GetErrno() {
    return errno;
}

void SetErrno(uintptr_t value) {
    errno = value;
}

