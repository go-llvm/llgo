package main

import (
	"testing"
)

func TestConst(t *testing.T) {
	err := runAndCompareMain(testdata("const.go"))
	if err != nil {t.Fatal(err)}
}

// vim: set ft=go:

