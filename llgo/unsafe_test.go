package main

import (
	"testing"
)

func TestUnsafePointer(t *testing.T) { checkOutputEqual(t, "unsafe/pointer.go") }
func TestSizeofStruct(t *testing.T)  { checkOutputEqual(t, "unsafe/sizeof_struct.go") }
func TestSizeofArray(t *testing.T)   { checkOutputEqual(t, "unsafe/sizeof_array.go") }
func TestConstSizeof(t *testing.T)   { checkOutputEqual(t, "unsafe/const_sizeof.go") }
