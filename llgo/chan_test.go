package main

import (
	"testing"
)

func TestChanBuffered(t *testing.T) { checkOutputEqual(t, "chan/buffered.go") }
func TestChanSelect(t *testing.T)   { checkOutputEqual(t, "chan/select.go") }

//func TestChanUnbuffered(t *testing.T) { checkOutputEqual(t, "chan/unbuffered.go") }
