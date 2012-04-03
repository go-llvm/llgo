package main

import (
	"testing"
)

func TestMultipleAssigment(t *testing.T) {
	err := runAndCompareMain(testdata("multi.go"))
	if err != nil {t.Fatal(err)}
}

// vim: set ft=go:

