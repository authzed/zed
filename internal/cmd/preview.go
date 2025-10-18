package cmd

import (
	"github.com/spf13/cobra"
)

func registerPreviewCmd(rootCmd *cobra.Command, schemaCompileCmd *cobra.Command) {
	previewCmd := &cobra.Command{
		Use:   "preview <subcommand>",
		Short: "Experimental commands that have been made available for preview",
	}

	schemaCmd := &cobra.Command{
		Use:        "schema <subcommand>",
		Short:      "Manage schema for a permissions system",
		Deprecated: "please use `zed schema compile`",
	}

	rootCmd.AddCommand(previewCmd)
	previewCmd.AddCommand(schemaCmd)
	schemaCmd.AddCommand(schemaCompileCmd)
}
