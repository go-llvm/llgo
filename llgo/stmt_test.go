package main

import (
	"testing"
)

func TestMultipleAssignment(t *testing.T)       { checkOutputEqual(t, "assignment/multi.go") }
func TestBinaryOperatorAssignment(t *testing.T) { checkOutputEqual(t, "assignment/binop.go") }
func TestNamedResultAssignment(t *testing.T)    { checkOutputEqual(t, "assignment/namedresult.go") }
func TestSwitchDefault(t *testing.T)            { checkOutputEqual(t, "switch/default.go") }
func TestSwitchEmpty(t *testing.T)              { checkOutputEqual(t, "switch/empty.go") }
func TestSwitchScope(t *testing.T)              { checkOutputEqual(t, "switch/scope.go") }
func TestSwitchBranching(t *testing.T)          { checkOutputEqual(t, "switch/branch.go") }
func TestIfLazy(t *testing.T)                   { checkOutputEqual(t, "if/lazy.go") }

// vim: set ft=go:
