package main

import (
	"testing"
)

func TestSliceLiteral(t *testing.T)   { checkOutputEqual(t, "slices/literal.go") }
func TestSliceAppend(t *testing.T)    { checkOutputEqual(t, "slices/append.go") }
func TestSliceMake(t *testing.T)      { checkOutputEqual(t, "slices/make.go") }
func TestSliceSliceExpr(t *testing.T) { checkOutputEqual(t, "slices/sliceexpr.go") }
func TestSliceCompare(t *testing.T)   { checkOutputEqual(t, "slices/compare.go") }
func TestSliceIndex(t *testing.T)     { checkOutputEqual(t, "slices/index.go") }
func TestSliceCopy(t *testing.T)      { checkOutputEqual(t, "slices/copy.go") }
func TestSliceCap(t *testing.T)       { checkOutputEqual(t, "slices/cap.go") }
