package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func registerPlugins(rootCmd *cobra.Command) {
	plugins := []struct{ name, description string }{
		{"testserver", "run a test server"},
	}
	for _, plugin := range plugins {
		binaryName := fmt.Sprintf("zed-%s", plugin.name)
		if commandIsAvailable(binaryName) {
			rootCmd.AddCommand(&cobra.Command{
				Use:                plugin.name,
				Short:              plugin.description,
				RunE:               pluginCmdFunc(binaryName),
				DisableFlagParsing: true, // Passes flags as args to the subcommand.
			})
		}
	}
}

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
