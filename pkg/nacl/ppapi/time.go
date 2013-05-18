// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ppapi

import (
	"time"
)

type ppTime float64

func (t ppTime) asTime() time.Time {
	seconds := int64(t)
	nanos := int64((t - ppTime(seconds)) * 1000000000)
	return time.Unix(seconds, int64(nanos))
}

type ppTimeTicks float64
