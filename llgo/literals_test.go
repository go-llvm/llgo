package main

import (
	"testing"
)

func TestLiteralSlice(t *testing.T) {
	err := runAndCheckMain(testdata("literals/slice.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLiteralStruct(t *testing.T) {
	err := runAndCheckMain(testdata("literals/struct.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

// vim: set ft=go:
