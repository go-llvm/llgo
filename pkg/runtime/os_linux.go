// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.
//
// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

// Atomically,
//  if(*addr == val) sleep
// Might be woken up spuriously; that's allowed.
// Don't sleep longer than ns; ns < 0 means forever.
func futexsleep(addr *uint32, val uint32, ns int64)

// If any procs are sleeping on addr, wake up at most cnt.
func futexwakeup(addr *uint32, cnt uint32)
