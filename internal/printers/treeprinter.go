package printers

import (
	"strings"

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

func (tp *TreePrinter) PrintIndented() {
	lines := strings.Split(tp.String(), "\n")
	indentedLines := make([]string, 0, len(lines))
	for _, line := range lines {
		indentedLines = append(indentedLines, "  "+line)
	}

	console.Println(strings.Join(indentedLines, "\n"))
}

func (tp *TreePrinter) String() string {
	return tp.tree.String()
}
