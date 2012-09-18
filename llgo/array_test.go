package main

import (
	"testing"
)

func TestArrayRange(t *testing.T) { checkOutputEqual(t, "arrays/range.go") }
func TestArrayIndex(t *testing.T) { checkOutputEqual(t, "arrays/index.go") }
func TestArraySlice(t *testing.T) { checkOutputEqual(t, "arrays/slice.go") }

// vim: set ft=go:
