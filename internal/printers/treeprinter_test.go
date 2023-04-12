package printers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTreePrinter(t *testing.T) {
	tp := NewTreePrinter()
	tp.Child("value")
	require.Equal(t, "value\n", tp.String())

	tp = NewTreePrinter()
	tp = tp.Child("parent")
	sib := tp.Child("child1")
	sib.Child("grandchild")
	tp.Child("child2")
	require.Equal(t, "parent\n├── child1\n│   └── grandchild\n└── child2\n", tp.String())
}
