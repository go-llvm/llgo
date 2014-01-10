package llgo

import (
	"github.com/axw/gollvm/llvm"
	"code.google.com/p/go.tools/go/types"
)

type abiArgInfo int

const (
	AIK_Direct = abiArgInfo(iota)
	AIK_Indirect
)

type backendType interface {
	ToLLVM(llvm.Context) llvm.Type
}

type ptrBType struct {
	target llvm.Type
}

func (t ptrBType) ToLLVM(c llvm.Context) llvm.Type {
	return llvm.PointerType(t.target, 0)
}

type intBType struct {
	width int
	signed bool
}

func (t intBType) ToLLVM(c llvm.Context) llvm.Type {
	return c.IntType(t.width)
}

type floatBType struct {
	isDouble bool
}

func (t floatBType) ToLLVM(c llvm.Context) llvm.Type {
	if t.isDouble {
		return c.DoubleType()
	} else {
		return c.FloatType()
	}
}

type structBType struct {
	fields []backendType
}

func (t structBType) ToLLVM(c llvm.Context) llvm.Type {
	var lfields []llvm.Type
	for _, f := range t.fields {
		lfields = append(lfields, f.ToLLVM(c))
	}
	return c.StructType(lfields, false)
}

type arrayBType struct {
	length int
	elem backendType
}

func (t arrayBType) ToLLVM(c llvm.Context) llvm.Type {
	return llvm.ArrayType(t.elem.ToLLVM(c), t.length)
}

// align returns the smallest y >= x such that y % a == 0.
func align(x, a int64) int64 {
	y := x + a - 1
	return y - y%a
}

func (tm *llvmTypeMap) sizeofStruct(fields ...types.Type) int64 {
	var o int64
	for _, f := range fields {
		a := tm.Alignof(f)
		o = align(o, a)
		o += tm.Sizeof(f)
	}
	return o
}

// This decides whether the x86_64 classification algorithm produces MEMORY for
// the given type. Given the subset of types that Go supports, this is exactly
// equivalent to testing the type's size.  See in particular the first step of
// the algorithm and its footnote.
func (tm *llvmTypeMap) classify(t ...types.Type) abiArgInfo {
	if tm.sizeofStruct(t...) > 16 {
		return AIK_Indirect
	} else {
		return AIK_Direct
	}
}

func (tm *llvmTypeMap) getBackendType(t types.Type) backendType {
	switch t := t.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.Bool, types.Uint8:
			return &intBType{8, false}
		case types.Int8:
			return &intBType{8, true}
		case types.Uint16:
			return &intBType{16, false}
		case types.Int16:
			return &intBType{16, true}
		case types.Uint32:
			return &intBType{32, false}
		case types.Int32:
			return &intBType{32, true}
		case types.Uint64:
			return &intBType{64, false}
		case types.Int64:
			return &intBType{64, true}
		case types.Uint, types.Uintptr:
			return &intBType{tm.target.PointerSize(), false}
		case types.Int:
			return &intBType{tm.target.PointerSize(), true}
		case types.Float32:
			return &floatBType{false}
		case types.Float64:
			return &floatBType{true}
		case types.UnsafePointer:
			return &ptrBType{llvm.Int8Type()}
		case types.Complex64:
			f32 := &floatBType{false}
			return &structBType{[]backendType{f32, f32}}
		case types.Complex128:
			f64 := &floatBType{true}
			return &structBType{[]backendType{f64, f64}}
		case types.String:
			return &structBType{[]backendType{&intBType{tm.target.PointerSize(), false}, &ptrBType{llvm.Int8Type()}}}
		}
	}

	panic("unhandled type")
}

func (tm *llvmTypeMap) expandType(argTypes []llvm.Type, argAttrs []llvm.Attribute, bt backendType) ([]llvm.Type, []llvm.Attribute) {
	switch bt := bt.(type) {
	case *structBType:
		// This isn't exactly right, but it will do for now.
		for _, f := range bt.fields {
			argTypes, argAttrs = tm.expandType(argTypes, argAttrs, f)
		}

	case *arrayBType:
		// Ditto.
		for i := 0; i != bt.length; i++ {
			argTypes, argAttrs = tm.expandType(argTypes, argAttrs, bt.elem)
		}

	default:
		argTypes = append(argTypes, bt.ToLLVM(tm.ctx))
		argAttrs = append(argAttrs, 0)
	}

	return argTypes, argAttrs
}

func (tm *llvmTypeMap) getFunctionType(args []types.Type, results []types.Type) (t llvm.Type, argAttrs []llvm.Attribute) {
	var returnType llvm.Type
	var argTypes []llvm.Type
	if len(results) == 0 {
		returnType = llvm.VoidType()
	} else {
		aik := tm.classify(results...)

		var resultsType llvm.Type
		if len(results) == 1 {
			resultsType = tm.ToLLVM(results[0])
		} else {
			elements := make([]llvm.Type, len(results))
			for i := range elements {
				elements[i] = tm.ToLLVM(results[i])
			}
			resultsType = llvm.StructType(elements, false)
		}

		switch aik {
		case AIK_Direct:
			returnType = resultsType

		case AIK_Indirect:
			returnType = llvm.VoidType()
			argTypes = []llvm.Type { llvm.PointerType(resultsType, 0) }
			argAttrs = []llvm.Attribute { llvm.StructRetAttribute }
		}
	}

	for _, arg := range args {
		aik := tm.classify(arg)

		switch aik {
		case AIK_Direct:
			bt := tm.getBackendType(arg)
			argTypes, argAttrs = tm.expandType(argTypes, argAttrs, bt)

		case AIK_Indirect:
			argTypes = append(argTypes, llvm.PointerType(tm.ToLLVM(arg), 0))
			argAttrs = append(argAttrs, llvm.ByValAttribute)
		}
	}

	t = llvm.FunctionType(returnType, argTypes, false)
	return
}
