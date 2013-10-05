package llgo

import (
	"code.google.com/p/go.tools/go/types"
	"github.com/axw/gollvm/llvm"
)

// Creates a new named variable allocated on the stack.
// If value is not nil, its contents is stored in the allocated stack space.
func (c *compiler) newStackVar(stackf *LLVMValue, v types.Object, value llvm.Value, name string) (stackvalue llvm.Value, stackvar *LLVMValue) {
	return c.newStackVarEx(0, stackf, v, value, name)
}

// Note that arguments are counted from 1, so that 1 is the 1st argument: http://llvm.org/docs/SourceLevelDebugging.html#local-variables
func (c *compiler) newArgStackVar(argument int, stackf *LLVMValue, v types.Object, value llvm.Value, name string) (stackvalue llvm.Value, stackvar *LLVMValue) {
	return c.newStackVarEx(argument, stackf, v, value, name)
}

func (c *compiler) newStackVarEx(argument int, stackf *LLVMValue, v types.Object, value llvm.Value, name string) (stackvalue llvm.Value, stackvar *LLVMValue) {
	typ := v.Type()

	// We need to put alloca instructions in the top block or the values
	// displayed when inspecting these variables in a debugger will be
	// completely messed up.
	curBlock := c.builder.GetInsertBlock()
	if p := curBlock.Parent(); !p.IsNil() {
		fb := p.FirstBasicBlock()
		fi := fb.FirstInstruction()
		if !fb.IsNil() && !fi.IsNil() {
			c.builder.SetInsertPointBefore(fi)
		}
	}
	old := c.builder.CurrentDebugLocation()
	c.builder.SetCurrentDebugLocation(llvm.Value{})
	stackvalue = c.builder.CreateAlloca(c.types.ToLLVM(typ), name)

	// For arguments we want to insert the store instruction
	// without debug information to ensure that they are executed
	// (and hence have proper values) before the debugger hits the
	// first line in a function.
	if argument == 0 {
		c.builder.SetCurrentDebugLocation(old)
		c.builder.SetInsertPointAtEnd(curBlock)
	}

	if !value.IsNil() {
		c.builder.CreateStore(value, stackvalue)
	}
	c.builder.SetCurrentDebugLocation(old)
	c.builder.SetInsertPointAtEnd(curBlock)

	ptrvalue := c.NewValue(stackvalue, types.NewPointer(typ))
	stackvar = ptrvalue.makePointee()
	stackvar.stack = stackf
	c.objectdata[v].Value = stackvar

	// Generate debug metadata (will return nil
	// if debug-data generation is disabled).
	if descriptor := c.createLocalVariableMetadata(v, argument); descriptor != nil {
		c.builder.InsertDeclare(
			c.module.Module,
			llvm.MDNode([]llvm.Value{stackvalue}),
			c.debug_info.MDNode(descriptor),
		)
	}
	return stackvalue, stackvar
}
