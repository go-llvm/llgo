package main

import (
	"testing"
)

/*
func _TestStaticStructInterfaceConversion(t *testing.T) {
	err := runAndCheckMain(testdata("interface.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

func _TestInterfaceToInterfaceConversion(t *testing.T) {
	err := runAndCheckMain(testdata("interface_i2i.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}
*/

func TestStaticBasicV2I(t *testing.T)   { checkOutputEqual(t, "interfaces/basic.go") }
func TestInterfaceMethods(t *testing.T) { checkOutputEqual(t, "interfaces/methods.go") }
func TestInterfaceAssert(t *testing.T) { checkOutputEqual(t, "interfaces/assert.go") }

// vim: set ft=go:
