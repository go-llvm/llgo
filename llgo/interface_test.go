package main

import (
	"testing"
)

func TestStaticStructInterfaceConversion(t *testing.T) {
	checkOutputEqual(t, "interfaces/static_conversion.go")
}
func TestInterfaceToInterfaceConversion(t *testing.T) {
	checkOutputEqual(t, "interfaces/i2i_conversion.go")
}
func TestStaticBasicV2I(t *testing.T)    { checkOutputEqual(t, "interfaces/basic.go") }
func TestInterfaceMethods(t *testing.T)  { checkOutputEqual(t, "interfaces/methods.go") }
func TestInterfaceAssert(t *testing.T)   { checkOutputEqual(t, "interfaces/assert.go") }
func TestError(t *testing.T)             { checkOutputEqual(t, "interfaces/error.go") }
func TestInterfaceWordSize(t *testing.T) { checkOutputEqual(t, "interfaces/wordsize.go") }
func TestCompareI2V(t *testing.T)        { checkOutputEqual(t, "interfaces/comparei2v.go") }
func TestCompareI2I(t *testing.T)        { checkOutputEqual(t, "interfaces/comparei2i.go") }

//func TestInterfaceImport(t *testing.T) { checkOutputEqual(t, "interfaces/import.go") }

// vim: set ft=go:
