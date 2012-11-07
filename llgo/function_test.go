package main

import (
	"testing"
)

func TestFunction(t *testing.T)        { checkOutputEqual(t, "fun.go") }
func TestVarargsFunction(t *testing.T) { checkOutputEqual(t, "varargs.go") }
func TestMethodSelectors(t *testing.T) { checkOutputEqual(t, "methods/selectors.go") }

// vim: set ft=go:
