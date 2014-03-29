package llgo

import (
	"code.google.com/p/go.tools/go/ssa"
	"code.google.com/p/go.tools/go/types"
	"github.com/go-llvm/llvm"
)

type byName []*ssa.Function

func (fns byName) Len() int { return len(fns) }
func (fns byName) Swap(i, j int) {
	fns[i], fns[j] = fns[j], fns[i]
}
func (fns byName) Less(i, j int) bool {
	return fns[i].Name() < fns[j].Name()
}

func (fr *frame) loadOrNull(cond, ptr llvm.Value, ty types.Type) *govalue {
	startbb := fr.builder.GetInsertBlock()
	loadbb := llvm.AddBasicBlock(fr.function, "")
	contbb := llvm.AddBasicBlock(fr.function, "")
	fr.builder.CreateCondBr(cond, loadbb, contbb)

	fr.builder.SetInsertPointAtEnd(loadbb)
	llty := fr.types.ToLLVM(ty)
	typedptr := fr.builder.CreateBitCast(ptr, llvm.PointerType(llty, 0), "")
	loadedval := fr.builder.CreateLoad(typedptr, "")
	fr.builder.CreateBr(contbb)

	fr.builder.SetInsertPointAtEnd(contbb)
	llv := fr.builder.CreatePHI(llty, "")
	llv.AddIncoming(
		[]llvm.Value{llvm.ConstNull(llty), loadedval},
		[]llvm.BasicBlock{startbb, loadbb},
	)
	return newValue(llv, ty)
}
