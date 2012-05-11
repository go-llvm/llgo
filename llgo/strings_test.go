package main

import (
	"testing"
)

func TestStringConcatenation(t *testing.T) {
	err := runAndCheckMain(testdata("strings/add.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStringComparison(t *testing.T) {
	err := runAndCheckMain(testdata("strings/compare.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

// vim: set ft=go:
