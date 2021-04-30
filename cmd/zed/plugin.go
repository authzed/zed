package main

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func commandIsAvailable(name string) bool {
	return exec.Command("command", "-v", name).Run() == nil
}

func pluginCmdFunc(binaryName string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		execCmd := exec.Command(binaryName, args...)
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr
		return execCmd.Run()
	}
}
