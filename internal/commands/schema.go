package commands

import (
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/console"
)

func RegisterSchemaCmd(rootCmd *cobra.Command) *cobra.Command {
	schemaCmd := &cobra.Command{
		Use:   "schema <subcommand>",
		Short: "Manage schema for a permissions system",
	}

	schemaReadCmd := &cobra.Command{
		Use:               "read",
		Short:             "Read the schema of a permissions system",
		Args:              ValidationWrapper(cobra.ExactArgs(0)),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE:              schemaReadCmdFunc,
	}

	rootCmd.AddCommand(schemaCmd)
	schemaCmd.AddCommand(schemaReadCmd)
	schemaReadCmd.Flags().Bool("json", false, "output as JSON")

	return schemaCmd
}

func schemaReadCmdFunc(cmd *cobra.Command, _ []string) error {
	client, err := client.NewClient(cmd)
	if err != nil {
		return err
	}
	request := &v1.ReadSchemaRequest{}
	log.Trace().Interface("request", request).Msg("requesting schema read")

	resp, err := client.ReadSchema(cmd.Context(), request)
	if err != nil {
		return err
	}

	if cobrautil.MustGetBool(cmd, "json") {
		prettyProto, err := PrettyProto(resp)
		if err != nil {
			return err
		}

		console.Println(string(prettyProto))
		return nil
	}

	console.Println(stringz.Join("\n\n", resp.SchemaText))
	return nil
}
