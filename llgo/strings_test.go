package main

import (
	"testing"
)

func TestStringConcatenation(t *testing.T) { checkOutputEqual(t, "strings/add.go") }
func TestStringComparison(t *testing.T)    { checkOutputEqual(t, "strings/compare.go") }
func TestStringIndex(t *testing.T)         { checkOutputEqual(t, "strings/index.go") }
