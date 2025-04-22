package printers

import (
	"strings"

	"github.com/xlab/treeprint"

	"github.com/authzed/zed/internal/console"
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

func (tp *TreePrinter) Indented() string {
	var sb strings.Builder
	lines := strings.Split(tp.String(), "\n")
	for _, line := range lines {
		sb.WriteString("  " + line + "\n")
	}

	return sb.String()
}

func (tp *TreePrinter) String() string {
	return tp.tree.String()
}
