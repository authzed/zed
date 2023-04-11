package printers

import (
	"github.com/authzed/zed/internal/console"

	"github.com/xlab/treeprint"
)

type TreePrinter struct {
	tree treeprint.Tree
}

func NewTreePrinter() *TreePrinter {
	return &TreePrinter{}
}

func (tp *TreePrinter) Child(val string) *TreePrinter {
	if tp.tree == nil {
		tp.tree = treeprint.NewWithRoot(val)
		return tp
	}
	return &TreePrinter{tree: tp.tree.AddBranch(val)}
}

func (tp *TreePrinter) Print() {
	console.Println(tp.String())
}

func (tp *TreePrinter) String() string {
	return tp.tree.String()
}
