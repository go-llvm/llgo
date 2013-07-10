package main

import "testing"

func TestRecursiveTypes(t *testing.T) { checkOutputEqual(t, "types/recursive.go") }
func TestNamedTypes(t *testing.T) { checkOutputEqual(t, "types/named.go") }
