package main

import (
	"testing"
)

func TestDereferencingAssignment(t *testing.T)  { checkOutputEqual(t, "assignment/dereferencing.go") }
func TestMultipleAssignment(t *testing.T)       { checkOutputEqual(t, "assignment/multi.go") }
func TestBinaryOperatorAssignment(t *testing.T) { checkOutputEqual(t, "assignment/binop.go") }
func TestNamedResultAssignment(t *testing.T)    { checkOutputEqual(t, "assignment/namedresult.go") }
func TestArrayAssignment(t *testing.T)          { checkOutputEqual(t, "assignment/arrays.go") }
func TestSwitchDefault(t *testing.T)            { checkOutputEqual(t, "switch/default.go") }
func TestSwitchEmpty(t *testing.T)              { checkOutputEqual(t, "switch/empty.go") }
func TestSwitchScope(t *testing.T)              { checkOutputEqual(t, "switch/scope.go") }
func TestSwitchBranching(t *testing.T)          { checkOutputEqual(t, "switch/branch.go") }
func TestSwitchStrings(t *testing.T)            { checkOutputEqual(t, "switch/strings.go") }
func TestTypeSwitch(t *testing.T)               { checkOutputEqual(t, "switch/type.go") }
func TestIfLazy(t *testing.T)                   { checkOutputEqual(t, "if/lazy.go") }
func TestGoto(t *testing.T)                     { checkOutputEqual(t, "branching/goto.go") }
func TestRecover(t *testing.T)                  { checkOutputEqual(t, "errors/recover.go") }
func TestLabeledBranching(t *testing.T)         { checkOutputEqual(t, "branching/labeled.go") }
func TestDefer(t *testing.T)                    { checkOutputEqual(t, "defer.go") }

// vim: set ft=go:
