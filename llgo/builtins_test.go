package main

import (
	"testing"
)

// Test use of the "new" builtin function.
func TestNew(t *testing.T) {
	err := runAndCheckMain(testdata("new.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

// vim: set ft=go:
