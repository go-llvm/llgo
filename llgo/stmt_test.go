package main

import (
	"testing"
)

func TestMultipleAssigment(t *testing.T) {
	err := runAndCheckMain(testdata("multi.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

// vim: set ft=go:
