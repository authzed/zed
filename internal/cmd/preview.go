package cmd

import (
	"github.com/spf13/cobra"
)

func registerPreviewCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(previewCmd)
}

var previewCmd = &cobra.Command{
	Use:   "preview <subcommand>",
	Short: "Experimental commands that have been made available for preview",
}
