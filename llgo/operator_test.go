package main

import (
	"testing"
)

func TestOperators(t *testing.T)         { checkOutputEqual(t, "operators/basics.go") }
func TestBinaryUntypedConversion(t *testing.T) { checkOutputEqual(t, "operators/binary_untyped.go") }
