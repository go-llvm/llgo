// Copyright 2012 The llgo Authors.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package llgo

import (
	"github.com/axw/gollvm/llvm"
	"go/ast"
)

type LabelData struct {
	Goto, Break, Continue llvm.BasicBlock
}

func (c *compiler) labelData(label *ast.Ident) *LabelData {
	data, ok := label.Obj.Data.(*LabelData)
	if !ok {
		bb := c.builder.GetInsertBlock()
		data = &LabelData{}
		data.Goto = llvm.AddBasicBlock(bb.Parent(), label.Name)
		label.Obj.Data = data
	}
	return data
}

func (c *compiler) VisitLabeledStmt(stmt *ast.LabeledStmt) {
	currBlock := c.builder.GetInsertBlock()
	labelData := c.labelData(stmt.Label)
	labelData.Goto.MoveAfter(currBlock)

	// If the inner statement is a for, switch or select statement, record
	// this label so we can update its Continue and Break blocks.
	switch stmt.Stmt.(type) {
	case *ast.ForStmt, *ast.SwitchStmt, *ast.SelectStmt, *ast.RangeStmt:
		c.lastlabel = stmt.Label
	}
	c.maybeImplicitBranch(labelData.Goto)
	c.builder.SetInsertPointAtEnd(labelData.Goto)
	c.VisitStmt(stmt.Stmt)
}
