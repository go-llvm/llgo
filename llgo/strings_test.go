package main

import (
	"testing"
)

func TestStringConcatenation(t *testing.T) { checkOutputEqual(t, "strings/add.go") }
func TestStringComparison(t *testing.T)    { checkOutputEqual(t, "strings/compare.go") }
func TestStringIndex(t *testing.T)         { checkOutputEqual(t, "strings/index.go") }
func TestStringSlice(t *testing.T)         { checkOutputEqual(t, "strings/slice.go") }
func TestStringBytes(t *testing.T)         { checkOutputEqual(t, "strings/bytes.go") }
func TestStringRange(t *testing.T)         { checkOutputEqual(t, "strings/range.go") }
func TestStringFromRune(t *testing.T)      { checkOutputEqual(t, "strings/runetostring.go") }
