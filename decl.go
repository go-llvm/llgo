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
)

func (c *compiler) VisitFuncProtoDecl(f *ast.FuncDecl) Value {
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
			pkgname := c.pkgmap[f.Name.Obj]
			fn_name = pkgname + "." + fn_name
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

func (c *compiler) VisitFuncDecl(f *ast.FuncDecl) Value {
	var fn Value
	if f.Name.Obj != nil {
		fn = c.Resolve(f.Name.Obj)
	} else {
		fn = c.VisitFuncProtoDecl(f)
	}
	if f.Body == nil {
		return fn
	}

	fn_type := fn.Type().(*types.Func)
	llvm_fn := fn.LLVMValue()

	entry := llvm.AddBasicBlock(llvm_fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)

	// Bind receiver, arguments and return values to their identifiers/objects.
	// We'll store each parameter on the stack so they're addressable.
	param_i := 0
	param_count := llvm_fn.ParamsCount()
	if f.Recv != nil {
		param_0 := llvm_fn.Param(0)
		recv_obj := fn_type.Recv
		recv_type := recv_obj.Type.(types.Type)
		stack_value := c.builder.CreateAlloca(
			c.types.ToLLVM(recv_type), recv_obj.Name)
		c.builder.CreateStore(param_0, stack_value)
		value := c.NewLLVMValue(stack_value, &types.Pointer{Base: recv_type})
		value.indirect = true
		recv_obj.Data = value
		param_i++
	}
	for _, param := range fn_type.Params {
		name := param.Name
		if name != "_" {
			param_type := param.Type.(types.Type)
			if fn_type.IsVariadic && param_i == param_count-1 {
				param_type = &types.Slice{Elt: param_type}
			}

			param_value := llvm_fn.Param(param_i)
			stack_value := c.builder.CreateAlloca(c.types.ToLLVM(param_type), name)
			c.builder.CreateStore(param_value, stack_value)
			value := c.NewLLVMValue(stack_value,
				&types.Pointer{Base: param_type})
			value.indirect = true
			param.Data = value
		}
		param_i++
	}

	c.functions = append(c.functions, fn)
	if f.Body != nil {
		c.VisitBlockStmt(f.Body)
	}
	c.functions = c.functions[0 : len(c.functions)-1]

	last_block := llvm_fn.LastBasicBlock()
	lasti := last_block.LastInstruction()
	if lasti.IsNil() || lasti.IsATerminatorInst().IsNil() {
		// Assume nil return type, AST should be checked first.
		c.builder.CreateRetVoid()
	}

	// Is it an 'init' function? Then record it.
	if f.Name.String() == "init" {
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
func (c *compiler) createGlobal(e ast.Expr,
	t types.Type,
	name string) (g *LLVMValue) {
	if e == nil {
		gv := llvm.AddGlobal(c.module.Module, c.types.ToLLVM(t), name)
		g = c.NewLLVMValue(gv, &types.Pointer{Base: t})
		g.indirect = !isArray(t)
		return g
	}

	if block := c.builder.GetInsertBlock(); !block.IsNil() {
		defer c.builder.SetInsertPointAtEnd(block)
	}
	fn_type := new(types.Func)
	llvm_fn_type := c.types.ToLLVM(fn_type).ElementType()
	fn := llvm.AddFunction(c.module.Module, "", llvm_fn_type)
	entry := llvm.AddBasicBlock(fn, "entry")
	c.builder.SetInsertPointAtEnd(entry)

	// Visit the expression. Dereference if necessary, and generalise
	// the type if one hasn't been specified.
	init_ := c.VisitExpr(e)
	isconst := false
	if value, isllvm := init_.(*LLVMValue); isllvm {
		if value.indirect {
			init_ = value.Deref()
		}
	} else {
		isconst = true
	}
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
	g = c.NewLLVMValue(gv, &types.Pointer{Base: t})
	g.indirect = !isArray(t)
	if isconst {
		// Initialiser is constant; discard function and return
		// global now.
		gv.SetInitializer(init_.LLVMValue())
	} else {
		gv.SetInitializer(llvm.ConstNull(c.types.ToLLVM(t)))
		c.builder.CreateStore(init_.LLVMValue(), gv)
	}

	if !fn.IsNil() {
		c.builder.CreateRetVoid()
		// FIXME order global ctors
		fn_value := c.NewLLVMValue(fn, fn_type)
		c.initfuncs = append(c.initfuncs, fn_value)
	}
	return g
}

func (c *compiler) VisitValueSpec(valspec *ast.ValueSpec, isconst bool) {
	var value_type types.Type
	if valspec.Type != nil {
		value_type = c.GetType(valspec.Type)
	}

	var iota_obj *ast.Object = types.Universe.Lookup("iota")
	defer func(data interface{}) {
		iota_obj.Data = data
	}(iota_obj.Data)

	for i, name_ := range valspec.Names {
		// We may resolve constants in the process of resolving others.
		obj := name_.Obj
		if _, isvalue := (obj.Data).(Value); isvalue {
			continue
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

		// Expression may have side-effects, so compute it regardless of
		// whether it'll be assigned to a name.
		var expr ast.Expr
		if i < len(valspec.Values) && valspec.Values[i] != nil {
			expr = valspec.Values[i]
		}

		// For constants, we just pass the ConstValue around. Otherwise, we
		// will convert it to an LLVMValue.
		var value Value
		name := name_.String()
		if !isconst {
			ispackagelevel := len(c.functions) == 0
			if !ispackagelevel {
				// Visit the expression.
				var init_ Value
				if expr != nil {
					init_ = c.VisitExpr(expr)
					if value, isllvm := init_.(*LLVMValue); isllvm {
						if value.indirect {
							init_ = value.Deref()
						}
					}
					if value_type == nil {
						value_type = init_.Type()
					}
				}

				// The variable should be allocated on the stack if it's
				// declared inside a function.
				var llvm_init llvm.Value
				stack_value := c.builder.CreateAlloca(
					c.types.ToLLVM(value_type), name)
				if init_ == nil {
					// If no initialiser was specified, set it to the
					// zero value.
					llvm_init = llvm.ConstNull(c.types.ToLLVM(value_type))
				} else {
					llvm_init = init_.Convert(value_type).LLVMValue()
				}
				c.builder.CreateStore(llvm_init, stack_value)
				llvm_value := c.NewLLVMValue(stack_value, &types.Pointer{Base: value_type})
				llvm_value.indirect = true
				value = llvm_value
			} else { // ispackagelevel
				// Set the initialiser. If it's a non-const value, then
				// we'll have to do the assignment in a global constructor
				// function.
				value = c.createGlobal(expr, value_type, name)
				if !name_.IsExported() {
					value.LLVMValue().SetLinkage(llvm.InternalLinkage)
				}
			}
		} else { // isconst
			value = c.VisitExpr(expr).(ConstValue)
			if value_type != nil {
				value = value.Convert(value_type)
			}
		}

		if name != "_" {
			obj.Data = value
		}
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
		for _, spec := range decl.Specs {
			valspec, _ := spec.(*ast.ValueSpec)
			c.VisitValueSpec(valspec, false)
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
