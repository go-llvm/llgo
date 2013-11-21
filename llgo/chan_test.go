package main

import (
	"testing"
)

func TestChanBuffered(t *testing.T) {
	checkOutputEqual(t, "chan/buffered.go")
}

func TestChanSelect(t *testing.T) {
	t.Skipf("channels not hooked up")
	checkOutputEqual(t, "chan/select.go")
}

func TestChanRange(t *testing.T) {
	checkOutputEqual(t, "chan/range.go")
}

//func TestChanUnbuffered(t *testing.T) { checkOutputEqual(t, "chan/unbuffered.go") }
