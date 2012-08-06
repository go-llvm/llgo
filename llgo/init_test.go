package main

import (
	"fmt"
	"testing"
)

// Test file-level var declarations.
func TestVarDecl(t *testing.T) { checkOutputEqual(t, "var.go") }

func TestInitFunctions(t *testing.T) {
	// There are two init functions, and their order is unspecified. So we just
	// want to check that sets {first two lines} for each execution are equal.
	check := func(a, b []string) error {
		if len(a) != len(b) {
			return fmt.Errorf("Lengths do not match: %q != %q", len(a), len(b))
		}
		if !((a[0] == b[0] && a[1] == b[1]) ||
			(a[0] == b[1] && a[1] == b[0])) {
			return fmt.Errorf("First two lines do not match: %q, %q", a, b)
		}
		return checkStringsEqual(a[2:], b[2:])
	}
	err := runAndCheckMain(check, testdata("init.go", "init2.go"))
	if err != nil {
		t.Fatal(err)
	}
}

// vim: set ft=go:
