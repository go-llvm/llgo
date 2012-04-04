package main

import (
	"testing"
)

func TestConst(t *testing.T) {
	err := runAndCheckMain(testdata("const.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

// vim: set ft=go:
