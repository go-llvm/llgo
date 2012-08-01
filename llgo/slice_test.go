package main

import (
	"testing"
)

func TestSliceLiteral(t *testing.T) {
	err := runAndCheckMain(testdata("slices/literal.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSliceAppend(t *testing.T) {
	err := runAndCheckMain(testdata("slices/append.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

// vim: set ft=go:
