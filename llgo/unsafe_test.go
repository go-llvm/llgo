package main

import (
	"testing"
)

func TestUnsafePointer(t *testing.T) { checkOutputEqual(t, "unsafe/pointer.go") }
func TestSizeofBasic(t *testing.T)   { checkOutputEqual(t, "unsafe/sizeof_basic.go") }
func TestSizeofStruct(t *testing.T)  { checkOutputEqual(t, "unsafe/sizeof_struct.go") }
func TestSizeofArray(t *testing.T)   { checkOutputEqual(t, "unsafe/sizeof_array.go") }
func TestConstSizeof(t *testing.T)   { checkOutputEqual(t, "unsafe/const_sizeof.go") }
func TestOffsetof(t *testing.T)      { checkOutputEqual(t, "unsafe/offsetof.go") }
