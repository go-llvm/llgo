package llgo

import (
	"code.google.com/p/go.tools/go/ssa"
)

type byName []*ssa.Function

func (fns byName) Len() int { return len(fns) }
func (fns byName) Swap(i, j int) {
	fns[i], fns[j] = fns[j], fns[i]
}
func (fns byName) Less(i, j int) bool {
	return fns[i].Name() < fns[j].Name()
}
