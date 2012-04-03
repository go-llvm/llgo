package main

import (
	"testing"
)

func TestNilComparison(t *testing.T) {
	err := runAndCompareMain(testdata("nil.go"))
	if err != nil {t.Fatal(err)}
}

// vim: set ft=go:

