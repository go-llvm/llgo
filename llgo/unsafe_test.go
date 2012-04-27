package main

import (
	"testing"
)

func TestUnsafePointer(t *testing.T) {
	err := runAndCheckMain(testdata("unsafe/pointer.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

// vim: set ft=go:
