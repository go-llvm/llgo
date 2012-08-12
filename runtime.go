package llgo

import (
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo/types"
	"go/ast"
	"go/parser"
	"go/token"
)

func (c *compiler) namedFunction(name string, signature string) llvm.Value {
	f := c.module.NamedFunction(name)
	if !f.IsNil() {
		return f
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", `package p;import("unsafe");`+signature+`{panic()}`, 0)
	if err != nil {
		panic(err)
	}

	files := map[string]*ast.File{"<src>": file}
	pkg, err := ast.NewPackage(fset, files, types.GcImport, types.Universe)
	if err != nil {
		panic(err)
	}

	_, err = types.Check(fset, pkg)
	if err != nil {
		panic(err)
	}

	fdecl := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
	ftype := fdecl.Name.Obj.Type.(*types.Func)
	return llvm.AddFunction(c.module.Module, name, c.types.ToLLVM(ftype).ElementType())
}
