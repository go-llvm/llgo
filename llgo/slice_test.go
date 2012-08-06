package main

import (
	"testing"
)

func TestSliceLiteral(t *testing.T) { checkOutputEqual(t, "slices/literal.go") }
func TestSliceAppend(t *testing.T)  { checkOutputEqual(t, "slices/append.go") }
