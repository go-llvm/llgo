package main

import (
	"testing"
)

func TestMapRange(t *testing.T) { checkOutputEqualUnordered(t, "maps/range.go") }

//func TestMapLiteral(t *testing.T) { checkOutputEqual(t, "maps/literal.go") }
func TestMapInsert(t *testing.T) { checkOutputEqual(t, "maps/insert.go") }
func TestMapDelete(t *testing.T) { checkOutputEqual(t, "maps/delete.go") }
func TestMapLookup(t *testing.T) { checkOutputEqual(t, "maps/lookup.go") }
