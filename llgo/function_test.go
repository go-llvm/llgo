package main

import (
	"testing"
)

func TestFunction(t *testing.T)          { checkOutputEqual(t, "fun.go") }
func TestVarargsFunction(t *testing.T)   { checkOutputEqual(t, "varargs.go") }
func TestMethodSelectors(t *testing.T)   { checkOutputEqual(t, "methods/selectors.go") }
func TestNilReceiverMethod(t *testing.T) { checkOutputEqual(t, "methods/nilrecv.go") }
func TestClosure(t *testing.T)           { checkOutputEqual(t, "closures/basic.go") }
func TestMultiValueCall(t *testing.T)    { checkOutputEqual(t, "functions/multivalue.go") }
func TestUnreachableCode(t *testing.T)   { checkOutputEqual(t, "functions/unreachable.go") }

// vim: set ft=go:
