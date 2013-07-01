package main

import (
	"testing"
)

func TestChanBuffered(t *testing.T) { checkOutputEqual(t, "chan/buffered.go") }

//func TestChanUnbuffered(t *testing.T) { checkOutputEqual(t, "chan/unbuffered.go") }
