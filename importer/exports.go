// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"fmt"
	"go/ast"
	"io"
	"math/big"
	"os"
	"path/filepath"

	"github.com/axw/llgo/build"

	"code.google.com/p/go.tools/go/exact"
	"code.google.com/p/go.tools/go/types"
)

type exporter struct {
	context   *build.Context
	writeFunc bool
	pkg       *types.Package
	writer    io.Writer
}

func (x *exporter) exportName(t interface{}) {
	switch t := t.(type) {
	case *types.Var:
		if t.Anonymous() || t.Name() == "" {
			x.write("? ")
		} else {
			x.write("%s ", t.Name())
		}
		x.exportName(t.Type())
	case exact.Value:
		switch t.Kind() {
		case exact.Float:
			x.exportName(&ast.BasicLit{Value: t.String()})
		default:
			x.write(t.String())
		}
	case *types.Basic:
		if t.Kind() == types.UnsafePointer {
			x.write("@\"unsafe\".Pointer")
		} else if t.Info()&types.IsUntyped == 0 {
			x.write(t.String())
		}
	case *types.TypeName:
		if t.Pkg() == nil {
			switch t.Name() {
			case "error":
				x.write("error")
				return
			case "Pointer":
				x.write("@\"unsafe\".Pointer")
				return
			}
		}
		if t.Pkg() == x.pkg || t.Pkg() == nil {
			x.write("@\"\".")
		} else {
			x.write("@\"%s\".", t.Pkg().Path())
		}
		x.write(t.Name())
	case *types.Named:
		x.exportName(t.Obj())
	case *types.Slice:
		x.write("[]")
		x.exportName(t.Elem())
	case *types.Pointer:
		x.write("*")
		x.exportName(t.Elem())
	case *types.Map:
		x.write("map[")
		x.exportName(t.Key())
		x.write("]")
		x.exportName(t.Elem())
	case *types.Chan:
		if t.Dir() == types.RecvOnly {
			x.write("<-")
		}
		x.write("chan")
		if t.Dir() == types.SendOnly {
			x.write("<-")
		}
		x.write(" ")
		x.exportName(t.Elem())
	case *ast.BasicLit:
		var b big.Rat

		fmt.Sscan(t.Value, &b)
		num, denom := b.Num(), b.Denom()

		if denom.Int64() == 1 {
			x.write("%d", num.Int64())
			return
		}

		// So yeah, this bit deserves a comment. We have a big.Rat number represented by its
		// numerator and denominator, and we want to turn this into a base 2 exponent form
		// where:
		//
		//			num / denom = x * 2^exp
		// 			(num / denum) / x = 2^exp
		// 			log2((num / denum) / x) = exp
		//
		// x and exp need to be integers as gc export data parses fractional numbers in the form of
		//
		// 		int_lit [ "p" int_lit ]
		//
		// Initially we just set x = 1 / denum turning the equation to
		//
		//			log2(num) = exp
		//
		// But we want exp to be an integer so it's rounded up, which is just the number of bits
		// needed to represent num.
		//
		// After this rounding however, x != 1 / denum, but we have the exact value of everything
		// else so we could just plug every known variable in:
		//
		//		num / denom = x * 2^exp
		//
		// But as exp is currently a positive value, x must be a fractional number which is
		// what we were trying to get rid of in the first place!
		//
		// So instead, we make x a large number that is divided by 2^exp which allows us to do this:
		//
		//		num / denom = x / 2^exp
		//		num / denom = x * 2^-exp
		//
		// And solving for x we get:
		//
		//		(num / denom) / 2^-exp = x
		//		(2^exp * num) / denom = x
		//
		// There's still a division in there though leading to a precision loss due to x and exp being constrained
		// to integer values. By making exp large enough we can add in precision that way. It needs to be at
		// least big enough to fit
		//
		//		newexp = log2(2^exp * denom) = log2(num) + log2(denom)
		//
		// and as it needs to be an integer value it also needs to be adjusted up to allow more fractional precision.
		//
		// I have no specific theoretical reason for choosing the fractional precision bits here, and it can be
		// changed if needed. One could say that the fractional accuracy in the final x number would be
		//
		// 		1/(2^fractional_accuracy_bits)
		//
		// and 23 is the number of fractional bits used by the IEEE_754-2008 binary32 format.
		const fractional_accuracy_bits = 23
		exp := num.BitLen() + denom.BitLen() + fractional_accuracy_bits
		exporter := x
		x := big.NewInt(2)
		x.Exp(x, big.NewInt(int64(exp)), nil)
		x.Mul(x, num)
		x.Div(x, denom)
		exporter.write("%dp-%d", x, exp)
	case *types.Tuple:
		for i := 0; i < t.Len(); i++ {
			if i > 0 {
				x.write(", ")
			}
			ta := t.At(i)
			n := ta.Name()
			if n == "" {
				n = "?"
			} else {
				n = `@"".` + n
			}
			x.write(n)
			x.write(" ")
			x.exportName(ta.Type())
		}
	case *types.Signature:
		if x.writeFunc {
			x.write("func")
		} else {
			x.writeFunc = true
		}
		x.write("(")
		if p := t.Params(); p != nil {
			if t.Variadic() {
				for i := 0; i < p.Len(); i++ {
					if i > 0 {
						x.write(", ")
					}
					ta := p.At(i)
					n := ta.Name()
					if n == "" {
						n = "?"
					} else {
						n = `@"".` + n
					}
					x.write(n)
					x.write(" ")
					ty := ta.Type()
					if i+1 == p.Len() {
						x.write("...")
						ty = ty.(*types.Slice).Elem()
					}
					x.exportName(ty)
				}
			} else {
				x.exportName(p)
			}
		}
		x.write(")")
		if r := t.Results(); r != nil {
			x.write("(")
			x.exportName(r)
			x.write(")")
		}
	case *types.Struct:
		x.write("struct { ")
		for i := 0; i < t.NumFields(); i++ {
			if i > 0 {
				x.write("; ")
			}
			f := t.Field(i)
			x.exportName(f)
		}
		x.write(" }")
	case *types.Array:
		x.write("[%d]", t.Len())
		x.exportName(t.Elem())
	case *types.Interface:
		x.write("interface { ")
		for i := 0; i < t.NumMethods(); i++ {
			if i > 0 {
				x.write("; ")
			}
			m := t.Method(i)
			x.write(m.Name())
			x.writeFunc = false
			x.exportName(m.Type())
		}
		x.write(" }")
	default:
		panic(fmt.Sprintf("UNHANDLED %T", t))
	}
}

func (x *exporter) write(a string, b ...interface{}) {
	if _, err := io.WriteString(x.writer, fmt.Sprintf(a, b...)); err != nil {
		panic(err)
	}
}

func (x *exporter) exportObject(obj types.Object) {
	switch t := obj.(type) {
	case *types.Var:
		if !obj.Exported() {
			return
		}
		x.write("\tvar @\"\".%s ", obj.Name())
		x.exportName(obj.Type())
		x.write("\n")
	case *types.Func:
		sig := t.Type().(*types.Signature)
		recv := sig.Recv()
		if recv == nil && !t.Exported() {
			return
			// The package "go/ast" has an interface "Decl" (http://golang.org/pkg/go/ast/#Decl)
			// containing "filtered or unexported methods", specifically a method named "declNode".
			//
			// No implementation of that method actually does anything, but it forces type
			// correctness and also disallows other packages from defining new types that
			// satisfies that interface.
			//
			// As the interface is exported and the types implementing the interface are too,
			// "declNode" must be exported to be able to properly type check any code that type
			// casts from and to that interface.
			//
			// To be clear; exporting *all* receiver methods is a superset of the methods
			// that must be exported.
		}
		x.write("\tfunc ")

		if recv != nil {
			x.write("(")
			x.exportName(recv)
			x.write(") ")
		}
		x.write(`@"".%s`, t.Name())
		x.writeFunc = false
		x.exportName(sig)
		x.write("\n")
	case *types.Const:
		if !t.Exported() {
			return
		}
		x.write("\tconst @\"\".%s ", t.Name())
		x.exportName(t.Type())
		x.write(" = ")
		x.exportName(t.Val())
		x.write("\n")
	case *types.TypeName:
		// Some types that are not exported outside of the package actually
		// need to be exported for the compiler.
		//
		// An example would be a package having an exported struct type
		// that has a member variable of some unexported type:
		//
		// As declaring variables of the exported struct type is allowed
		// outside of the package, the compiler needs to know the size
		// of the struct.
		//
		// To be clear; exporting *all* types is a superset of the types
		// that must be exported.
		x.write("\ttype @\"\".%s ", obj.Name())
		x.exportName(obj.Type().Underlying())
		x.write("\n")
		if u, ok := obj.Type().(*types.Named); ok {
			for i := 0; i < u.NumMethods(); i++ {
				m := u.Method(i)
				x.exportObject(m)
			}
		}
	default:
		panic(fmt.Sprintf("UNHANDLED %T", t))
	}
}

func (x *exporter) export(pkg *types.Package) error {
	x.pkg = pkg
	x.writeFunc = true
	exportsFile := packageExportsFile(x.context, pkg.Path())
	err := os.MkdirAll(filepath.Dir(exportsFile), 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}
	f2, err := os.Create(exportsFile)
	if err != nil {
		return err
	}
	defer f2.Close()
	x.writer = f2
	x.write("package %s\n", pkg.Name())
	for _, imp := range pkg.Imports() {
		x.write("\timport %s \"%s\"\n", imp.Name(), imp.Path())
	}
	for _, n := range pkg.Scope().Names() {
		if obj := pkg.Scope().Lookup(n); obj != nil {
			x.exportObject(obj)
		}
	}
	x.write("$$")
	return nil
}

// Export generates a file containing package export data
// suitable for importing with Importer.Import.
func Export(ctx *build.Context, pkg *types.Package) error {
	x := &exporter{context: ctx}
	return x.export(pkg)
}
