package main

import (
	"testing"
)

func TestFunction(t *testing.T) {
	err := runAndCheckMain(testdata("fun.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

func TestVarargsFunction(t *testing.T) {
	err := runAndCheckMain(testdata("varargs.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

// vim: set ft=go:
