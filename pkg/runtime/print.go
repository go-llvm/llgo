// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package runtime

// runtime.printfloat, below, is based on code from
// go/pkg/runtime/print.c
//
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// printfloat formats a float64 into the given buffer,
// returning the number of bytes required to represent
// the value.
func printfloat(v float64) string {
	switch {
	case isNaN(v):
		return "NaN"
	case v == posinf:
		return "+Inf"
	case v == neginf:
		return "-Inf"
	}

	var buf [20]byte
	var e, s, i, n int32
	var h float64
	n = 7 // digits printed
	e = 0 // exp
	s = 0 // sign
	if v != 0 {
		// sign
		if v < 0 {
			v = -v
			s = 1
		}

		// normalize
		for v >= 10 {
			e++
			v /= 10
		}
		for v < 1 {
			e--
			v *= 10
		}

		// round
		h = 5
		for i = 0; i < n; i++ {
			h /= 10
		}

		v += h
		if v >= 10 {
			e++
			v /= 10
		}
	}

	// format +d.dddd+edd
	buf[0] = '+'
	if s != 0 {
		buf[0] = '-'
	}
	for i = 0; i < n; i++ {
		s = int32(v)
		buf[i+2] = byte(s + '0')
		v -= float64(s)
		v *= 10.
	}
	buf[1] = buf[2]

	buf[2] = '.'

	buf[n+2] = 'e'
	buf[n+3] = '+'
	if e < 0 {
		e = -e
		buf[n+3] = '-'
	}

	buf[n+4] = byte((e / 100) + '0')
	buf[n+5] = byte((e/10)%10 + '0')
	buf[n+6] = byte((e % 10) + '0')
	return string(buf[:n+7])
}
