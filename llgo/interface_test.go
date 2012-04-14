package main

import (
	"testing"
)

func TestStaticStructInterfaceConversion(t *testing.T) {
	err := runAndCheckMain(testdata("interface.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

func TestInterfaceToInterfaceConversion(t *testing.T) {
	err := runAndCheckMain(testdata("interface_i2i.go"), checkStringsEqual)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStaticBasicTypeToInterfaceConversion(t *testing.T) {
    err := runAndCheckMain(testdata("interface_basic.go"), checkStringsEqual)
    if err != nil {
        t.Fatal(err)
    }
}

// vim: set ft=go:
