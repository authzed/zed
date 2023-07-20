package commands

import "os"

var isFileTerminal = func(f *os.File) bool { return true }

func performBulkDeletionConfirmation(counter int) error {
	// Nothing to do.
	return nil
}
