// Modifications copyright 2011, 2012 Andrew Wilkins <axwalk@gmail.com>.

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements operations on ideal constants.

package types

import (
	"go/token"
	"math/big"
	"strconv"
)

// TODO(gri) Consider changing the API so Const is an interface
//           and operations on consts don't have to type switch.

// A Const implements an ideal constant Value.
// The zero value z for a Const is not a valid constant value.
type Const struct {
	// representation of constant values:
	// ideal bool     ->  bool
	// ideal int      ->  *big.Int
	// ideal float    ->  *big.Rat
	// ideal complex  ->  Cmplx
	// ideal string   ->  string
	Val interface{}
}

// Representation of complex values.
type Cmplx struct {
	Re, Im *big.Rat
}

func assert(cond bool) {
	if !cond {
		panic("go/types internal error: assertion failed")
	}
}

// MakeConst makes an ideal constant from a literal
// token and the corresponding literal string.
func MakeConst(tok token.Token, lit string) Const {
	switch tok {
	case token.INT:
		var x big.Int
		_, ok := x.SetString(lit, 0)
		assert(ok)
		return Const{&x}
	case token.FLOAT:
		var y big.Rat
		_, ok := y.SetString(lit)
		assert(ok)
		return Const{&y}
	case token.IMAG:
		assert(lit[len(lit)-1] == 'i')
		var im big.Rat
		_, ok := im.SetString(lit[0 : len(lit)-1])
		assert(ok)
		return Const{Cmplx{big.NewRat(0, 1), &im}}
	case token.CHAR:
		assert(lit[0] == '\'' && lit[len(lit)-1] == '\'')
		code, _, _, err := strconv.UnquoteChar(lit[1:len(lit)-1], '\'')
		assert(err == nil)
		return Const{big.NewInt(int64(code))}
	case token.STRING:
		s, err := strconv.Unquote(lit)
		assert(err == nil)
		return Const{s}
	}
	panic("unreachable")
}

// MakeZero returns the zero constant for the given type.
func MakeZero(typ *Type) Const {
	// TODO(gri) fix this
	return Const{0}
}

// Match attempts to match the internal constant representations of x and y.
// If the attempt is successful, the result is the values of x and y,
// if necessary converted to have the same internal representation; otherwise
// the results are invalid.
func (x Const) Match(y Const) (u, v Const) {
	switch a := x.Val.(type) {
	case bool:
		if _, ok := y.Val.(bool); ok {
			u, v = x, y
		}
	case *big.Int:
		switch y.Val.(type) {
		case *big.Int:
			u, v = x, y
		case *big.Rat:
			var z big.Rat
			z.SetInt(a)
			u, v = Const{&z}, y
		case Cmplx:
			var z big.Rat
			z.SetInt(a)
			u, v = Const{Cmplx{&z, big.NewRat(0, 1)}}, y
		}
	case *big.Rat:
		switch y.Val.(type) {
		case *big.Int:
			v, u = y.Match(x)
		case *big.Rat:
			u, v = x, y
		case Cmplx:
			u, v = Const{Cmplx{a, big.NewRat(0, 0)}}, y
		}
	case Cmplx:
		switch y.Val.(type) {
		case *big.Int, *big.Rat:
			v, u = y.Match(x)
		case Cmplx:
			u, v = x, y
		}
	case string:
		if _, ok := y.Val.(string); ok {
			u, v = x, y
		}
	default:
		panic("unreachable")
	}
	return
}

// Convert attempts to convert the constant x to a given type.
// If the attempt is successful, the result is the new constant;
// otherwise the result is invalid.
func (x Const) Convert(typ *Type) Const {
	// TODO(gri) implement this
	switch x := x.Val.(type) {
	//case bool:
	case *big.Int:
		switch Underlying(*typ) {
		case Float32, Float64:
			var z big.Rat
			z.SetInt(x)
			return Const{&z}
		case String:
			return Const{string(x.Int64())}
		case Complex64, Complex128:
			var z big.Rat
			z.SetInt(x)
			return Const{Cmplx{&z, &big.Rat{}}}
		}
	case *big.Rat:
		switch Underlying(*typ) {
		case Byte, Int, Uint, Int8, Uint8, Int16, Uint16, Int32, Uint32, Int64, Uint64:
			// Convert to an integer. Remove the fractional component.
			num, denom := x.Num(), x.Denom()
			var z big.Int
			z.Quo(num, denom)
			return Const{&z}
		}
		//case Cmplx:
		//case string:
	}
	//panic("unimplemented")
	return x
}

func (x Const) String() string {
	switch x := x.Val.(type) {
	case nil:
		return "nil"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case *big.Int:
		return x.String()
	case *big.Rat:
		// 10 digits of precision after decimal point seems fine
		return x.FloatString(10)
	case Cmplx:
		// TODO(gri) don't print 0 components
		return x.Re.FloatString(10) + " + " + x.Im.FloatString(10) + "i"
	case string:
		return x
	}
	panic("unreachable")
}

func (x Const) UnaryOp(op token.Token) Const {
	var z interface{}
	switch x := x.Val.(type) {
	case bool:
		z = unaryBoolOp(x, op)
	case *big.Int:
		z = unaryIntOp(x, op)
	case *big.Rat:
		z = unaryFloatOp(x, op)
	case Cmplx:
		z = unaryCmplxOp(x, op)
	default:
		panic("unreachable")
	}
	return Const{z}
}

func unaryBoolOp(x bool, op token.Token) interface{} {
	switch op {
	case token.NOT:
		return !x
	}
	panic("unreachable")
}

func unaryIntOp(x *big.Int, op token.Token) interface{} {
	var z big.Int
	switch op {
	case token.ADD:
		return z.Set(x)
	case token.SUB:
		return z.Neg(x)
	case token.XOR:
		return z.Not(x)
	}
	panic("unreachable")
}

func unaryFloatOp(x *big.Rat, op token.Token) interface{} {
	var z big.Rat
	switch op {
	case token.ADD:
		return z.Set(x)
	case token.SUB:
		return z.Neg(x)
	}
	panic("unreachable")
}

func unaryCmplxOp(x Cmplx, op token.Token) interface{} {
	switch op {
	case token.ADD:
	case token.SUB:
	}
	panic("unimplemented")
}

func (x Const) BinaryOp(op token.Token, y Const) Const {
	var z interface{}
	switch x := x.Val.(type) {
	case bool:
		z = binaryBoolOp(x, op, y.Val.(bool))
	case *big.Int:
		z = binaryIntOp(x, op, y.Val.(*big.Int))
	case *big.Rat:
		z = binaryFloatOp(x, op, y.Val.(*big.Rat))
	case Cmplx:
		z = binaryCmplxOp(x, op, y.Val.(Cmplx))
	case string:
		z = binaryStringOp(x, op, y.Val.(string))
	default:
		panic("unreachable")
	}
	return Const{z}
}

func binaryBoolOp(x bool, op token.Token, y bool) interface{} {
	switch op {
	case token.EQL:
		return x == y
	case token.NEQ:
		return x != y
	case token.LAND:
		return x && y
	case token.LOR:
		return x || y
	}
	panic("unreachable")
}

func binaryIntOp(x *big.Int, op token.Token, y *big.Int) interface{} {
	var z big.Int
	switch op {
	case token.ADD:
		return z.Add(x, y)
	case token.SUB:
		return z.Sub(x, y)
	case token.MUL:
		return z.Mul(x, y)
	case token.QUO:
		return z.Quo(x, y)
	case token.REM:
		return z.Rem(x, y)
	case token.AND:
		return z.And(x, y)
	case token.OR:
		return z.Or(x, y)
	case token.XOR:
		return z.Xor(x, y)
	case token.AND_NOT:
		return z.AndNot(x, y)
	case token.SHL:
		// The shift length must be uint, or untyped int and
		// convertible to uint.
		// TODO 32/64bit
		if y.BitLen() > 32 {
			panic("Excessive shift length")
		}
		return z.Lsh(x, uint(y.Int64()))
	case token.SHR:
		if y.BitLen() > 32 {
			panic("Excessive shift length")
		}
		return z.Rsh(x, uint(y.Int64()))
	case token.EQL:
		return x.Cmp(y) == 0
	case token.NEQ:
		return x.Cmp(y) != 0
	case token.LSS:
		return x.Cmp(y) < 0
	case token.LEQ:
		return x.Cmp(y) <= 0
	case token.GTR:
		return x.Cmp(y) > 0
	case token.GEQ:
		return x.Cmp(y) >= 0
	}
	panic("unreachable")
}

func binaryFloatOp(x *big.Rat, op token.Token, y *big.Rat) interface{} {
	var z big.Rat
	switch op {
	case token.ADD:
		return z.Add(x, y)
	case token.SUB:
		return z.Sub(x, y)
	case token.MUL:
		return z.Mul(x, y)
	case token.QUO:
		return z.Quo(x, y)
	case token.EQL:
		return x.Cmp(y) == 0
	case token.NEQ:
		return x.Cmp(y) != 0
	case token.LSS:
		return x.Cmp(y) < 0
	case token.LEQ:
		return x.Cmp(y) <= 0
	case token.GTR:
		return x.Cmp(y) > 0
	case token.GEQ:
		return x.Cmp(y) >= 0
	}
	panic("unreachable")
}

func binaryCmplxOp(x Cmplx, op token.Token, y Cmplx) interface{} {
	a, b := x.Re, x.Im
	c, d := y.Re, y.Im
	switch op {
	case token.ADD:
		// (a+c) + i(b+d)
		var re, im big.Rat
		re.Add(a, c)
		im.Add(b, d)
		return Cmplx{&re, &im}
	case token.SUB:
		// (a-c) + i(b-d)
		var re, im big.Rat
		re.Sub(a, c)
		im.Sub(b, d)
		return Cmplx{&re, &im}
	case token.MUL:
		// (ac-bd) + i(bc+ad)
		var ac, bd, bc, ad big.Rat
		ac.Mul(a, c)
		bd.Mul(b, d)
		bc.Mul(b, c)
		ad.Mul(a, d)
		var re, im big.Rat
		re.Sub(&ac, &bd)
		im.Add(&bc, &ad)
		return Cmplx{&re, &im}
	case token.QUO:
		// (ac+bd)/s + i(bc-ad)/s, with s = cc + dd
		var ac, bd, bc, ad, s big.Rat
		ac.Mul(a, c)
		bd.Mul(b, d)
		bc.Mul(b, c)
		ad.Mul(a, d)
		s.Add(c.Mul(c, c), d.Mul(d, d))
		var re, im big.Rat
		re.Add(&ac, &bd)
		re.Quo(&re, &s)
		im.Sub(&bc, &ad)
		im.Quo(&im, &s)
		return Cmplx{&re, &im}
	case token.EQL:
		return a.Cmp(c) == 0 && b.Cmp(d) == 0
	case token.NEQ:
		return a.Cmp(c) != 0 || b.Cmp(d) != 0
	}
	panic("unreachable")
}

func binaryStringOp(x string, op token.Token, y string) interface{} {
	switch op {
	case token.ADD:
		return x + y
	case token.EQL:
		return x == y
	case token.NEQ:
		return x != y
	case token.LSS:
		return x < y
	case token.LEQ:
		return x <= y
	case token.GTR:
		return x > y
	case token.GEQ:
		return x >= y
	}
	panic("unreachable")
}

// vim: set ft=go :
