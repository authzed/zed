package cmd

import (
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/commands"
	"github.com/authzed/zed/internal/mcp"
)

func registerMCPCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.AddCommand(mcpRunCmd)

	mcpRunCmd.Flags().IntP("port", "p", 9999, "port for the HTTP streaming server")
}

var mcpCmd = &cobra.Command{
	Use:   "mcp <subcommand>",
	Short: "MCP (Model Context Protocol) server commands",
	Long: `MCP (Model Context Protocol) server commands.
	
The MCP server provides tooling and resources for developing and debugging SpiceDB schema and relationships. The server runs an in-memory development instance of SpiceDB and does not connect to a running instance of SpiceDB.

To use with Claude Code, run "zed mcp run" to start the SpiceDB Dev MCP server and then run "claude mcp add --transport http spicedb http://localhost:9999/mcp" to add the server to your Claude Code integrations.
`,
}

var mcpRunCmd = &cobra.Command{
	Use:               "run",
	Short:             "Run the MCP server",
	Args:              commands.ValidationWrapper(cobra.ExactArgs(0)),
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              mcpRunCmdFunc,
}

func mcpRunCmdFunc(cmd *cobra.Command, _ []string) error {
	port, _ := cmd.Flags().GetInt("port")

	server := mcp.NewSpiceDBMCPServer()
	return server.Run(port)
}
