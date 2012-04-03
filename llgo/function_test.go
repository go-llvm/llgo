package main

import (
	"testing"
)

func TestFunction(t *testing.T) {
	err := runAndCompareMain(testdata("fun.go"))
	if err != nil {t.Fatal(err)}
}

func TestVarargsFunction(t *testing.T) {
	err := runAndCompareMain(testdata("varargs.go"))
	if err != nil {t.Fatal(err)}
}

// vim: set ft=go:

