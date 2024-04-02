package commands

import "os"

var isFileTerminal = func(f *os.File) bool { return true }
