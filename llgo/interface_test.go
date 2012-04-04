package main

import (
	"testing"
)

func TestStaticStructInterfaceConversion(t *testing.T) {
	err := runAndCheckMain(testdata("interface.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

// vim: set ft=go:
