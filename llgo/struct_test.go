package main

import (
	"testing"
)

func TestCircularType(t *testing.T) {
	err := runAndCompareMain(testdata("circulartype.go"))
	if err != nil {t.Fatal(err)}
}

// vim: set ft=go:

