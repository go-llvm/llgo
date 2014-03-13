// Copyright 2011 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package debug

// DwarfLang is a type for DW_LANG_XXX constants.
type DwarfLang uint32

const (
	// http://dwarfstd.org/ShowIssue.php?issue=101014.1&type=open
	DW_LANG_Go DwarfLang = 0x0016
)

// DwarfTypeEncoding is a type for DW_ATE_XXX type encoding constants.
type DwarfTypeEncoding uint32

const (
	DW_ATE_address         DwarfTypeEncoding = 0x01
	DW_ATE_boolean         DwarfTypeEncoding = 0x02
	DW_ATE_complex_float   DwarfTypeEncoding = 0x03
	DW_ATE_float           DwarfTypeEncoding = 0x04
	DW_ATE_signed          DwarfTypeEncoding = 0x05
	DW_ATE_signed_char     DwarfTypeEncoding = 0x06
	DW_ATE_unsigned        DwarfTypeEncoding = 0x07
	DW_ATE_unsigned_char   DwarfTypeEncoding = 0x08
	DW_ATE_imaginary_float DwarfTypeEncoding = 0x09
	DW_ATE_packed_decimal  DwarfTypeEncoding = 0x0a
	DW_ATE_numeric_string  DwarfTypeEncoding = 0x0b
	DW_ATE_edited          DwarfTypeEncoding = 0x0c
	DW_ATE_signed_fixed    DwarfTypeEncoding = 0x0d
	DW_ATE_unsigned_fixed  DwarfTypeEncoding = 0x0e
	DW_ATE_decimal_float   DwarfTypeEncoding = 0x0f
	DW_ATE_UTF             DwarfTypeEncoding = 0x10
	DW_ATE_lo_user         DwarfTypeEncoding = 0x80
	DW_ATE_hi_user         DwarfTypeEncoding = 0xff
)
