package main

import (
	"testing"
)

func TestNilComparison(t *testing.T) {
	err := runAndCheckMain(testdata("nil.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

// vim: set ft=go:
