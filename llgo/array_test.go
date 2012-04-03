package main

import (
	"testing"
)

// Test array initialisation and iteration.
func TestArray(t *testing.T) {
	err := runAndCompareMain(testdata("array.go"))
	if err != nil {t.Fatal(err)}
}

// vim: set ft=go:

