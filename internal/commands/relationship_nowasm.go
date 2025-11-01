//go:build !wasm

package commands

import (
	"os"

	"golang.org/x/term"
)

var isFileTerminal = func(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }
