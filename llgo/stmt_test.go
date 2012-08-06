package main

import (
	"testing"
)

func TestMultipleAssigment(t *testing.T) {
	err := runAndCheckMain(testdata("multi.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSwitchDefaultClause(t *testing.T) {
	err := runAndCheckMain(testdata("switch/default.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEmptySwitch(t *testing.T) {
	err := runAndCheckMain(testdata("switch/empty.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSwitchScope(t *testing.T) {
	err := runAndCheckMain(testdata("switch/scope.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

func TestIfLazy(t *testing.T) {
	err := runAndCheckMain(testdata("if/lazy.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBinaryOperatorAssignment(t *testing.T) {
	err := runAndCheckMain(testdata("assignment/binop.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

// vim: set ft=go:
