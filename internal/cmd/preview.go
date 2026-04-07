package cmd

import (
	"github.com/spf13/cobra"
)

func registerPreviewCmd(rootCmd *cobra.Command) {
	previewCmd := &cobra.Command{
		Use:   "preview <subcommand>",
		Short: "Experimental commands that have been made available for preview",
	}

	rootCmd.AddCommand(previewCmd)
}
