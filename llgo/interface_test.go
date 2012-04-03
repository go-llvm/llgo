package main

import (
	"testing"
)

func TestStaticStructInterfaceConversion(t *testing.T) {
	err := runAndCompareMain(testdata("interface.go"))
	if err != nil {t.Fatal(err)}
}

// vim: set ft=go:

