//go:build !wasm
// +build !wasm

package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
)

func performBulkDeletionConfirmation(counter int) error {
	message := fmt.Sprintf("Will delete %d relationships. Continue?", counter)
	if counter > 1000 {
		message = "Will delete 1000+ relationships. Continue?"
	}
	if counter < 0 {
		message = "Will delete all matching relationships. Continue?"
	}

	response := false
	err := survey.AskOne(&survey.Confirm{
		Message: message,
	}, &response)
	if err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			os.Exit(0)
		}

		return err
	}

	if !response {
		os.Exit(1)
	}
	return nil
}
