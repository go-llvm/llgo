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
    "go/ast"
    "go/token"
    "go/types"
    "reflect"
    "strconv"
    "github.com/axw/gollvm/llvm"
)

func (c *compiler) VisitFuncProtoDecl(f *ast.FuncDecl) Value {
    // Get the function type. The function type will be cached in f.Name.Obj,
    // so check that first.
    var fn_type Type
    if f.Name.Obj != nil {
        if result, isvalue := f.Name.Obj.Data.(Value); isvalue {
            return result
        }
        fn_type = c.ObjGetType(f.Name.Obj)
    }
    if fn_type == nil {
        fn_type = c.VisitFuncType(f.Type)
    }
    fn_name := f.Name.String()

    // Make "init" functions anonymous.
    if fn_name == "init" {fn_name = ""}
    fn := llvm.AddFunction(c.module.Module, fn_name, fn_type.LLVMType())
    if c.module.Name == "main" && fn_name == "main" {
        fn.SetLinkage(llvm.ExternalLinkage)
    }

    // llvm.AddFunction returns a pointer-to-function, so change the
    // result type to a Pointer.
    fn_ptr_type := &Pointer{Base: fn_type}
    result := NewLLVMValue(c.builder, fn, fn_ptr_type)
    if f.Name.Obj != nil {
        f.Name.Obj.Data = result
        f.Name.Obj.Type = fn_ptr_type
    }
    return result
}

func (c *compiler) VisitFuncDecl(f *ast.FuncDecl) Value {
    name := f.Name.String()
    fn, _ := c.Lookup(name)
    if fn == nil {fn = c.VisitFuncProtoDecl(f)}

    fn_type := Deref(fn.Type()).(*Func)
    llvm_fn := fn.LLVMValue()

    // Bind receiver, arguments and return values to their identifiers/objects.
    param_i := 0
    if f.Recv != nil {
        param_0 := llvm_fn.Param(0)
        recv_obj := fn_type.Recv
        //recv_obj := f.Recv.List[0].Names[0].Obj
        recv_type := recv_obj.Type.(Type)
        recv_obj.Data = NewLLVMValue(c.builder, param_0, recv_type)
        param_i++
    }
    if param_i < len(fn_type.Params) {
        for _, param := range fn_type.Params {
            name := param.Name
            param_type := param.Type.(Type)
            value := llvm_fn.Param(param_i)
            value.SetName(name)
            if name != "_" {
                param.Data = NewLLVMValue(c.builder, value, param_type)
            }
            param_i++
        }
    }

    entry := llvm.AddBasicBlock(llvm_fn, "entry")
    c.builder.SetInsertPointAtEnd(entry)

    c.functions = append(c.functions, fn)
    if f.Body != nil {c.VisitBlockStmt(f.Body)}
    c.functions = c.functions[0:len(c.functions)-1]

    last_block := llvm_fn.LastBasicBlock()
    lasti := last_block.LastInstruction()
    if lasti.IsNil() || lasti.InstructionOpcode() != llvm.Ret {
        // Assume nil return type, AST should be checked first.
        c.builder.CreateRetVoid()
    }

    // Is it an 'init' function? Then record it.
    if name == "init" {
        c.initfuncs = append(c.initfuncs, fn)
    } else {
        //if obj != nil {
        //    obj.Data = fn
        //}
    }
    return fn
}

func (c *compiler) VisitValueSpec(valspec *ast.ValueSpec, isconst bool) {
    var value_type Type
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
        if _, isvalue := (obj.Data).(Value); isvalue {continue}

        // Set iota if necessary.
        if isconst {
            if iota_, isint := (name_.Obj.Data).(int); isint {
                iota_value := NewConstValue(token.INT, strconv.Itoa(iota_))
                iota_value.typ.Kind = UntypedInt
                iota_obj.Data = iota_value

                // Con objects with an iota have an embedded ValueSpec
                // in the Decl field. We'll just pull it out and use it
                // for evaluating the expression below.
                valspec, _ = (name_.Obj.Decl).(*ast.ValueSpec)
            }
        }

        // Expression may have side-effects, so compute it regardless of
        // whether it'll be assigned to a name.
        var value Value
        if valspec.Values != nil && i < len(valspec.Values) &&
           valspec.Values[i] != nil {
            value = c.VisitExpr(valspec.Values[i])
        }

        name := name_.String()
        if name != "_" {
            // For constants, we just pass the ConstValue around. Otherwise, we
            // will convert it to an LLVMValue.
            if !isconst {
                // TODO (from language spec)
                // If the type is absent and the corresponding expression evaluates to
                // an untyped constant, the type of the declared variable is bool, int,
                // float64, or string respectively, depending on whether the value is
                // a boolean, integer, floating-point, or string constant.
                if value_type == nil {
                    value_type = value.Type()
                }

                ispackagelevel := len(c.functions) == 0
                if !ispackagelevel {
                    // The variable should be allocated on the stack if it's
                    // declared inside a function.
                    init_ := value
                    var llvm_init llvm.Value
                    stack_value := c.builder.CreateAlloca(
                        value_type.LLVMType(), name)
                    if init_ == nil {
                        // If no initialiser was specified, set it to the
                        // zero value.
                        llvm_init = llvm.ConstNull(value_type.LLVMType())
                    } else {
                        llvm_init = init_.Convert(value_type).LLVMValue()
                    }
                    c.builder.CreateStore(llvm_init, stack_value)
                    value_type = &Pointer{Base: value_type}
                    llvm_value := NewLLVMValue(c.builder, stack_value, value_type)
                    llvm_value.indirect = true
                    value = llvm_value
                } else {
                    exported := name_.IsExported()

                    // If it's a non-string constant, assign it to .
                    var constprim bool
                    if _, isconstval := value.(ConstValue); isconstval {
                        if basic, isbasic := (value.Type()).(*Basic); isbasic {
                            constprim = basic.Kind != String
                        }
                    }

                    if isconst && constprim && !exported {
                        // Not exported, and it's a constant. Let's forego creating
                        // the internal constant and just pass around the
                        // llvm.Value.
                        obj.Kind = ast.Con // Change to constant
                        obj.Data = value.Convert(value_type)
                    } else {
                        init_ := value
                        global_value := llvm.AddGlobal(
                            c.module.Module, value_type.LLVMType(), name)
                        if init_ != nil {
                            init_ = init_.Convert(value_type)
                            global_value.SetInitializer(init_.LLVMValue())
                        }
                        if isconst {
                            global_value.SetGlobalConstant(true)
                        }
                        if !exported {
                            global_value.SetLinkage(llvm.InternalLinkage)
                        }

                        value = NewLLVMValue(c.builder, global_value, value_type)
                        obj.Data = value
        
                        // If it's not an array, we should mark the value as being
                        // "indirect" (i.e. it must be loaded before use).
                        // TODO
                        //if value_type.TypeKind() != llvm.ArrayTypeKind {
                        //    setindirect(value)
                        //}
                    }
                }
            }
            obj.Data = value
        }
    }
}

func (c *compiler) VisitTypeSpec(spec *ast.TypeSpec) {
    obj := spec.Name.Obj
    type_, istype := (obj.Type).(Type)
    if !istype {
        type_ = c.GetType(spec.Type)
        obj.Type = type_
    }
}

func (c *compiler) VisitImportSpec(spec *ast.ImportSpec) {
    // TODO we will need to create our own Importer.
    path, err := strconv.Unquote(spec.Path.Value)
    if err != nil {panic(err)}
    pkg, err := types.GcImporter(c.imports, path)
    if err != nil {panic(err)}

    // TODO handle spec.Name (local package name), if not nil

    // Insert the package object into the scope.
    c.filescope.Outer.Insert(pkg)
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
    switch x := decl.(type) {
    case *ast.FuncDecl: return c.VisitFuncDecl(x)
    case *ast.GenDecl: {
        c.VisitGenDecl(x)
        return nil
    }
    }
    panic(fmt.Sprintf("Unhandled decl (%s) at %s\n",
                      reflect.TypeOf(decl),
                      c.fileset.Position(decl.Pos())))
}

// vim: set ft=go :

