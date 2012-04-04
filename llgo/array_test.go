package main

import (
	"testing"
)

// Test array initialisation and iteration.
func TestArray(t *testing.T) {
	err := runAndCheckMain(testdata("array.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

// vim: set ft=go:
