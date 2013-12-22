// Copyright 2013 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"bufio"
	"bytes"
	"code.google.com/p/go.tools/go/exact"
	"code.google.com/p/go.tools/go/gcimporter"
	"code.google.com/p/go.tools/go/types"
	"fmt"
	"go/ast"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
)

var (
	importsRe = regexp.MustCompile(`import \w+ "([\w/]+)"`)
)

type (
	importer struct {
		compiler  *compiler
		myimports map[string]*types.Package
	}
	exporter struct {
		writeFunc bool
		pkg       *types.Package
		compiler  *compiler
		writer    io.Writer
	}
)

func (c *compiler) packageExportsFile(path string) string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = runtime.GOROOT()
	} else {
		gopath = filepath.SplitList(gopath)[0]
	}
	return filepath.Join(gopath, "pkg", "llgo", c.TargetTriple, path+".lgx")
}

func (c *importer) Import(imports map[string]*types.Package, path string) (pkg *types.Package, err error) {
	if c.myimports == nil {
		// myimports is different from imports. Imports can contain
		// dummy packages not loaded by us, while myimports will
		// be "pure".
		c.myimports = make(map[string]*types.Package)
	}
	if pkg, ok := c.myimports[path]; ok {
		if pkg == nil {
			return nil, fmt.Errorf("Previous attempt at loading package failed")
		}
		return pkg, nil
	}

	var data []byte
	pkgfile := c.compiler.packageExportsFile(path)
	if path == "unsafe" {
		// Importing these packages have issues
		//
		// unsafe:
		// 		If this is actually imported, go.types will panic due to invalid type conversions.
		//		This because it is a built in package  (http://tip.golang.org/doc/spec#Package_unsafe)
		// 		and thus shouldn't be treated as a normal package anyway.
	} else {
		data, _ = ioutil.ReadFile(pkgfile)
	}

	if data != nil {
		// Need to load dependencies first
		for _, match := range importsRe.FindAllStringSubmatch(string(data), -1) {
			if _, ok := c.myimports[match[1]]; !ok {
				_, err := c.Import(imports, match[1])
				if err != nil {
					return nil, err
				}
			}
		}
		pkg, err = gcimporter.ImportData(imports, pkgfile, path, bufio.NewReader(bytes.NewBuffer(data)))
	}
	if pkg == nil || err != nil {
		if data != nil {
			return nil, fmt.Errorf("Failed to load package %s: %v", path, err)
		}
		// Package has not been compiled yet, so fall back to
		// the standard GcImport.
		pkg, err = gcimporter.Import(imports, path)
	}

	c.myimports[path] = pkg
	imports[path] = pkg

	return pkg, err
}

func (c *exporter) exportName(t interface{}) {
	switch t := t.(type) {
	case *types.Var:
		if t.Anonymous() || t.Name() == "" {
			c.write("? ")
		} else {
			c.write("%s ", t.Name())
		}
		c.exportName(t.Type())
	case exact.Value:
		switch t.Kind() {
		case exact.Float:
			c.exportName(&ast.BasicLit{Value: t.String()})
		default:
			c.write(t.String())
		}
	case *types.Basic:
		if t.Kind() == types.UnsafePointer {
			c.write("@\"unsafe\".Pointer")
		} else if t.Info()&types.IsUntyped == 0 {
			c.write(t.String())
		}
	case *types.TypeName:
		if t.Pkg() == nil {
			switch t.Name() {
			case "error":
				c.write("error")
				return
			case "Pointer":
				c.write("@\"unsafe\".Pointer")
				return
			}
		}
		if t.Pkg() == c.pkg || t.Pkg() == nil {
			c.write("@\"\".")
		} else {
			c.write("@\"%s\".", t.Pkg().Path())
		}
		c.write(t.Name())
	case *types.Named:
		c.exportName(t.Obj())
	case *types.Slice:
		c.write("[]")
		c.exportName(t.Elem())
	case *types.Pointer:
		c.write("*")
		c.exportName(t.Elem())
	case *types.Map:
		c.write("map[")
		c.exportName(t.Key())
		c.write("]")
		c.exportName(t.Elem())
	case *types.Chan:
		if t.Dir() == types.RecvOnly {
			c.write("<-")
		}
		c.write("chan")
		if t.Dir() == types.SendOnly {
			c.write("<-")
		}
		c.write(" ")
		c.exportName(t.Elem())
	case *ast.BasicLit:
		var b big.Rat

		fmt.Sscan(t.Value, &b)
		num, denom := b.Num(), b.Denom()

		if denom.Int64() == 1 {
			c.write("%d", num.Int64())
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
		x := big.NewInt(2)
		x.Exp(x, big.NewInt(int64(exp)), nil)
		x.Mul(x, num)
		x.Div(x, denom)
		c.write("%dp-%d", x, exp)
	case *types.Tuple:
		for i := 0; i < t.Len(); i++ {
			if i > 0 {
				c.write(", ")
			}
			ta := t.At(i)
			n := ta.Name()
			if n == "" {
				n = "?"
			} else {
				n = `@"".` + n
			}
			c.write(n)
			c.write(" ")
			c.exportName(ta.Type())
		}
	case *types.Signature:
		if c.writeFunc {
			c.write("func")
		} else {
			c.writeFunc = true
		}
		c.write("(")
		if p := t.Params(); p != nil {
			if t.IsVariadic() {
				for i := 0; i < p.Len(); i++ {
					if i > 0 {
						c.write(", ")
					}
					ta := p.At(i)
					n := ta.Name()
					if n == "" {
						n = "?"
					} else {
						n = `@"".` + n
					}
					c.write(n)
					c.write(" ")
					ty := ta.Type()
					if i+1 == p.Len() {
						c.write("...")
						ty = ty.(*types.Slice).Elem()
					}
					c.exportName(ty)
				}
			} else {
				c.exportName(p)
			}
		}
		c.write(")")
		if r := t.Results(); r != nil {
			c.write("(")
			c.exportName(r)
			c.write(")")
		}
	case *types.Struct:
		c.write("struct { ")
		for i := 0; i < t.NumFields(); i++ {
			if i > 0 {
				c.write("; ")
			}
			f := t.Field(i)
			c.exportName(f)
		}
		c.write(" }")
	case *types.Array:
		c.write("[%d]", t.Len())
		c.exportName(t.Elem())
	case *types.Interface:
		c.write("interface { ")
		for i := 0; i < t.NumMethods(); i++ {
			if i > 0 {
				c.write("; ")
			}
			m := t.Method(i)
			c.write(m.Name())
			c.writeFunc = false
			c.exportName(m.Type())
		}
		c.write(" }")
	default:
		panic(fmt.Sprintf("UNHANDLED %T", t))
	}
}

func (c *exporter) write(a string, b ...interface{}) {
	if _, err := io.WriteString(c.writer, fmt.Sprintf(a, b...)); err != nil {
		panic(err)
	}
}

func (c *exporter) exportObject(obj types.Object) {
	switch t := obj.(type) {
	case *types.Var:
		if !obj.IsExported() {
			return
		}
		c.write("\tvar @\"\".%s ", obj.Name())
		c.exportName(obj.Type())
		c.write("\n")
	case *types.Func:
		sig := t.Type().(*types.Signature)
		recv := sig.Recv()
		if recv == nil && !t.IsExported() {
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
		c.write("\tfunc ")

		if recv != nil {
			c.write("(")
			c.exportName(recv)
			c.write(") ")
		}
		c.write(`@"".%s`, t.Name())
		c.writeFunc = false
		c.exportName(sig)
		c.write("\n")
	case *types.Const:
		if !t.IsExported() {
			return
		}
		c.write("\tconst @\"\".%s ", t.Name())
		c.exportName(t.Type())
		c.write(" = ")
		c.exportName(t.Val())
		c.write("\n")
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
		c.write("\ttype @\"\".%s ", obj.Name())
		c.exportName(obj.Type().Underlying())
		c.write("\n")
		if u, ok := obj.Type().(*types.Named); ok {
			for i := 0; i < u.NumMethods(); i++ {
				m := u.Method(i)
				c.exportObject(m)
			}
		}
	default:
		panic(fmt.Sprintf("UNHANDLED %T", t))
	}

}

func (c *exporter) Export(pkg *types.Package) error {
	c.pkg = pkg
	c.writeFunc = true
	f2, err := os.Create(c.compiler.packageExportsFile(pkg.Path()))
	if err != nil {
		return err
	}
	defer f2.Close()
	c.writer = f2
	c.write("package %s\n", pkg.Name())
	for _, imp := range c.pkg.Imports() {
		c.write("\timport %s \"%s\"\n", imp.Name(), imp.Path())
	}

	for _, n := range pkg.Scope().Names() {
		if obj := pkg.Scope().Lookup(n); obj != nil {
			c.exportObject(obj)
		}
	}

	c.write("$$")
	return nil
}
