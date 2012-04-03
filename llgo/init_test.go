package main

import (
	"testing"
)

// Test file-level var declarations.
func TestVarDecl(t *testing.T) {
	err := runAndCompareMain(testdata("var.go"))
	if err != nil {t.Fatal(err)}
}

func TestInitFunctions(t *testing.T) {
	err := runAndCompareMain(testdata("init.go", "init2.go"))
	if err != nil {t.Fatal(err)}
}

// vim: set ft=go:

