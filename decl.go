/*
Copyright (c) 2011, 2012 Andrew Wilkins <axwalk@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package llgo

import (
	"fmt"
	"github.com/axw/gollvm/llvm"
	"github.com/axw/llgo/types"
	"go/ast"
	"go/scanner"
	"go/token"
	"reflect"
	"strconv"
	"strings"
)

func (c *compiler) VisitFuncProtoDecl(f *ast.FuncDecl) *LLVMValue {
	var fn_type *types.Func
	fn_name := f.Name.String()

	exported := ast.IsExported(fn_name)
	if fn_name == "init" {
		// Make "init" functions anonymous.
		fn_name = ""
		// "init" functions aren't recorded by the parser, so f.Name.Obj is
		// not set.
		fn_type = &types.Func{ /* no params or result */}
	} else {
		fn_type = f.Name.Obj.Type.(*types.Func)
		if c.module.Name == "main" && fn_name == "main" {
			exported = true
		} else {
			if fn_type.Recv != nil {
				recv := fn_type.Recv
				if recvtyp, ok := recv.Type.(*types.Pointer); ok {
					recv = recvtyp.Base.(*types.Name).Obj
				} else {
					recv = recv.Type.(*types.Name).Obj
				}
				pkgname := c.pkgmap[recv]
				fn_name = pkgname + "." + recv.Name + "." + fn_name
			} else {
				pkgname := c.pkgmap[f.Name.Obj]
				fn_name = pkgname + "." + fn_name
			}
		}
	}

	llvm_fn_type := c.types.ToLLVM(fn_type).ElementType()
	fn := llvm.AddFunction(c.module.Module, fn_name, llvm_fn_type)
	if exported {
		fn.SetLinkage(llvm.ExternalLinkage)
	}

	result := c.NewLLVMValue(fn, fn_type)
	if f.Name.Obj != nil {
		f.Name.Obj.Data = result
		f.Name.Obj.Type = fn_type
	}
	return result
}

// buildFunction takes a function Value, a list of parameters, and a body,
// and generates code for the function.
func (c *compiler) buildFunction(f *LLVMValue, params []*ast.Object, body *ast.BlockStmt) {
	ftyp := f.Type().(*types.Func)
	llvm_fn := f.LLVMValue()
	entry := llvm.AddBasicBlock(llvm_fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)

	// Bind receiver, arguments and return values to their identifiers/objects.
	// We'll store each parameter on the stack so they're addressable.
	for i, obj := range params {
		if obj.Name != "" {
			value := llvm_fn.Param(i)
			typ := obj.Type.(types.Type)
			stackvalue := c.builder.CreateAlloca(c.types.ToLLVM(typ), obj.Name)
			c.builder.CreateStore(value, stackvalue)
			ptrvalue := c.NewLLVMValue(stackvalue, &types.Pointer{Base: typ})
			obj.Data = ptrvalue.makePointee()
		}
	}

	// Allocate space on the stack for named results.
	for _, obj := range ftyp.Results {
		if obj.Name != "" {
			typ := obj.Type.(types.Type)
			llvmtyp := c.types.ToLLVM(typ)
			stackptr := c.builder.CreateAlloca(llvmtyp, obj.Name)
			c.builder.CreateStore(llvm.ConstNull(llvmtyp), stackptr)
			ptrvalue := c.NewLLVMValue(stackptr, &types.Pointer{Base: typ})
			obj.Data = ptrvalue.makePointee()
		}
	}

	c.functions = append(c.functions, f)
	c.VisitBlockStmt(body, false)
	c.functions = c.functions[0 : len(c.functions)-1]
	last := llvm_fn.LastBasicBlock()
	if in := last.LastInstruction(); in.IsNil() || in.IsATerminatorInst().IsNil() {
		// Assume nil return type, AST should be checked first.
		c.builder.SetInsertPointAtEnd(last)
		c.builder.CreateRetVoid()
	}
}

func (c *compiler) VisitFuncDecl(f *ast.FuncDecl) Value {
	var fn *LLVMValue
	if f.Name.Obj != nil {
		fn = c.Resolve(f.Name.Obj).(*LLVMValue)
	} else {
		fn = c.VisitFuncProtoDecl(f)
	}
	if f.Body == nil {
		return fn
	}

	fn_type := fn.Type().(*types.Func)
	paramObjects := fn_type.Params
	if f.Recv != nil {
		paramObjects = append([]*ast.Object{fn_type.Recv}, paramObjects...)
	}
	c.buildFunction(fn, paramObjects, f.Body)

	// Is it an 'init' function? Then record it.
	if f.Name.Name == "init" {
		c.initfuncs = append(c.initfuncs, fn)
	} else {
		//if obj != nil {
		//    obj.Data = fn
		//}
	}
	return fn
}

func isArray(t types.Type) bool {
	_, isarray := t.(*types.Array)
	return isarray
}

// Create a constructor function which initialises a global.
// TODO collapse all global inits into one init function?
func (c *compiler) createGlobal(e ast.Expr, t types.Type, name string, export bool) (g *LLVMValue) {
	if e == nil {
		llvmtyp := c.types.ToLLVM(t)
		gv := llvm.AddGlobal(c.module.Module, llvmtyp, name)
		if !export {
			gv.SetLinkage(llvm.PrivateLinkage)
		}
		gv.SetInitializer(llvm.ConstNull(llvmtyp))
		g = c.NewLLVMValue(gv, &types.Pointer{Base: t})
		if !isArray(t) {
			return g.makePointee()
		}
		return g
	}

	if block := c.builder.GetInsertBlock(); !block.IsNil() {
		defer c.builder.SetInsertPointAtEnd(block)
	}
	fn_type := new(types.Func)
	llvm_fn_type := c.types.ToLLVM(fn_type).ElementType()
	fn := llvm.AddFunction(c.module.Module, "", llvm_fn_type)
	fn.SetLinkage(llvm.PrivateLinkage)
	entry := llvm.AddBasicBlock(fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)

	// Visit the expression. Dereference if necessary, and generalise
	// the type if one hasn't been specified.
	init_ := c.VisitExpr(e)
	_, isconst := init_.(ConstValue)
	if t == nil {
		t = init_.Type()
	} else {
		init_ = init_.Convert(t)
	}

	// If the result is a constant, discard the function before
	// creating any llvm.Value's.
	if isconst {
		fn.EraseFromParentAsFunction()
		fn = llvm.Value{nil}
	}

	// Set the initialiser. If it's a non-const value, then
	// we'll have to do the assignment in a global constructor
	// function.
	gv := llvm.AddGlobal(c.module.Module, c.types.ToLLVM(t), name)
	if !export {
		gv.SetLinkage(llvm.PrivateLinkage)
	}
	g = c.NewLLVMValue(gv, &types.Pointer{Base: t})
	if !isArray(t) {
		g = g.makePointee()
	}
	if isconst {
		// Initialiser is constant; discard function and return global now.
		gv.SetInitializer(init_.LLVMValue())
	} else {
		gv.SetInitializer(llvm.ConstNull(c.types.ToLLVM(t)))
		c.builder.CreateStore(init_.LLVMValue(), gv)
	}

	if !fn.IsNil() {
		c.builder.CreateRetVoid()
		// FIXME order global ctors
		fn_value := c.NewLLVMValue(fn, fn_type)
		c.varinitfuncs = append(c.varinitfuncs, fn_value)
	}
	return g
}

func (c *compiler) VisitValueSpec(valspec *ast.ValueSpec, isconst bool) {
	var iota_obj *ast.Object = types.Universe.Lookup("iota")
	defer func(data interface{}) {
		iota_obj.Data = data
	}(iota_obj.Data)

	var value_type types.Type
	for i, name_ := range valspec.Names {
		// We may resolve constants in the process of resolving others.
		obj := name_.Obj
		if _, isvalue := (obj.Data).(Value); isvalue {
			continue
		}

		// Expression may have side-effects, so compute it regardless of
		// whether it'll be assigned to a name.
		var expr ast.Expr
		if i < len(valspec.Values) && valspec.Values[i] != nil {
			expr = valspec.Values[i]
		}

		// Set iota if necessary.
		if isconst {
			if iota_, isint := (name_.Obj.Data).(int); isint {
				iota_value := c.NewConstValue(token.INT, strconv.Itoa(iota_))
				iota_obj.Data = iota_value

				// Con objects with an iota have an embedded ValueSpec
				// in the Decl field. We'll just pull it out and use it
				// for evaluating the expression below.
				valspec = (name_.Obj.Decl).(*ast.ValueSpec)
			}
		}

		// If the name is "_", then we can just evaluate the expression
		// and ignore the result. We handle package level variables
		// specially, below.
		pkgname, ispackagelevel := c.pkgmap[name_.Obj]
		name := name_.String()
		if name == "_" && !ispackagelevel {
			if expr != nil {
				c.VisitExpr(expr)
			}
			continue
		} else {
			if value_type == nil && valspec.Type != nil {
				value_type = c.GetType(valspec.Type)
			}
		}

		// For constants, we just pass the ConstValue around. Otherwise, we
		// will convert it to an LLVMValue.
		var value Value
		if !isconst {
			if !ispackagelevel {
				// Visit the expression.
				var init_ Value
				if expr != nil {
					init_ = c.VisitExpr(expr)
					if value_type == nil {
						value_type = init_.Type()
					}
				}

				// The variable should be allocated on the stack if it's
				// declared inside a function.
				var llvm_init llvm.Value
				stack_value := c.builder.CreateAlloca(c.types.ToLLVM(value_type), name)
				if init_ == nil {
					// If no initialiser was specified, set it to the
					// zero value.
					llvm_init = llvm.ConstNull(c.types.ToLLVM(value_type))
				} else {
					llvm_init = init_.Convert(value_type).LLVMValue()
				}
				c.builder.CreateStore(llvm_init, stack_value)
				llvm_value := c.NewLLVMValue(stack_value, &types.Pointer{Base: value_type})
				value = llvm_value.makePointee()
			} else { // ispackagelevel
				// Set the initialiser. If it's a non-const value, then
				// we'll have to do the assignment in a global constructor
				// function.
				export := name_.IsExported()
				value = c.createGlobal(expr, value_type, pkgname+"."+name, export)
			}
		} else { // isconst
			value = c.VisitExpr(expr).(ConstValue)
			if value_type != nil {
				value = value.Convert(value_type)
			}
		}
		obj.Data = value
	}
}

func (c *compiler) VisitTypeSpec(spec *ast.TypeSpec) {
	obj := spec.Name.Obj
	type_, istype := (obj.Type).(types.Type)
	if !istype {
		type_ = c.GetType(spec.Type)
		if name, isname := type_.(*types.Name); isname {
			type_ = types.Underlying(name)
		}
		obj.Type = &types.Name{Underlying: type_, Obj: obj}
	}
}

func (c *compiler) VisitImportSpec(spec *ast.ImportSpec) {
	/*
	   path, err := strconv.Unquote(spec.Path.Value)
	   if err != nil {panic(err)}
	   imported := c.pkg.Imports[path]
	   pkg, err := types.GcImporter(c.imports, path)
	   if err != nil {panic(err)}

	   // TODO handle spec.Name (local package name), if not nil
	   // Insert the package object into the scope.
	   c.filescope.Outer.Insert(pkg)
	*/
}

func (c *compiler) VisitGenDecl(decl *ast.GenDecl) {
	switch decl.Tok {
	case token.IMPORT:
		for _, spec := range decl.Specs {
			importspec, _ := spec.(*ast.ImportSpec)
			c.VisitImportSpec(importspec)
		}
	case token.TYPE:
		for _, spec := range decl.Specs {
			typespec, _ := spec.(*ast.TypeSpec)
			c.VisitTypeSpec(typespec)
		}
	case token.CONST:
		for _, spec := range decl.Specs {
			valspec, _ := spec.(*ast.ValueSpec)
			c.VisitValueSpec(valspec, true)
		}
	case token.VAR:
		// Global variable attributes
		// TODO only parse attributes for package-level var's.
		var attributes []Attribute
		if attributes == nil && decl.Doc != nil {
			attributes = make([]Attribute, 0)
			for _, comment := range decl.Doc.List {
				text := comment.Text[2:]
				if strings.HasPrefix(comment.Text, "/*") {
					text = text[:len(text)-2]
				}
				attr := parseAttribute(strings.TrimSpace(text))
				if attr != nil {
					attributes = append(attributes, attr)
				}
			}
		}
		for _, spec := range decl.Specs {
			valspec, _ := spec.(*ast.ValueSpec)
			c.VisitValueSpec(valspec, false)
			for _, attr := range attributes {
				for _, name := range valspec.Names {
					attr.Apply(name.Obj.Data.(Value))
				}
			}
		}
	}
}

func (c *compiler) VisitDecl(decl ast.Decl) Value {
	// This is temporary. We'll return errors later, rather than panicking.
	if c.logger != nil {
		c.logger.Println("Compile declaration:", c.fileset.Position(decl.Pos()))
	}
	defer func() {
		if e := recover(); e != nil {
			elist := new(scanner.ErrorList)
			elist.Add(c.fileset.Position(decl.Pos()), fmt.Sprint(e))
			panic(elist)
		}
	}()

	switch x := decl.(type) {
	case *ast.FuncDecl:
		return c.VisitFuncDecl(x)
	case *ast.GenDecl:
		c.VisitGenDecl(x)
		return nil
	}
	panic(fmt.Sprintf("Unhandled decl (%s) at %s\n",
		reflect.TypeOf(decl),
		c.fileset.Position(decl.Pos())))
}

// vim: set ft=go :
