package main

import (
	"testing"
)

func TestCircularType(t *testing.T) {
	err := runAndCheckMain(testdata("circulartype.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

// vim: set ft=go:
