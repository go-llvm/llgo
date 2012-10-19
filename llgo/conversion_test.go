package main

import (
	"testing"
)

func TestConvertSameUnderlying(t *testing.T)   { checkOutputEqual(t, "conversions/sameunderlying.go") }
func TestConvertFloatToInt(t *testing.T)   { checkOutputEqual(t, "conversions/float.go") }
