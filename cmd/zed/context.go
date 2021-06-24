package main

import (
	"os"

	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/printers"
	"github.com/authzed/zed/internal/storage"
)

func registerContextCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(contextCmd)

	contextCmd.AddCommand(contextListCmd)
	contextListCmd.Flags().Bool("reveal-tokens", false, "display secrets in results")

	contextCmd.AddCommand(contextSetCmd)
	contextCmd.AddCommand(contextRemoveCmd)
	contextCmd.AddCommand(contextUseCmd)
}

var contextCmd = &cobra.Command{
	Use:               "context <subcommand>",
	Short:             "manage your machines Authzed credentials",
	PersistentPreRunE: persistentPreRunE,
}

var contextListCmd = &cobra.Command{
	Use:               "list",
	Short:             "list all contexts",
	Args:              cobra.ExactArgs(0),
	PersistentPreRunE: persistentPreRunE,
	RunE:              contextListCmdFunc,
}

var contextSetCmd = &cobra.Command{
	Use:               "set <system> <token>",
	Short:             "create or overwrite a context",
	Args:              cobra.ExactArgs(2),
	PersistentPreRunE: persistentPreRunE,
	RunE:              contextSetCmdFunc,
}

var contextRemoveCmd = &cobra.Command{
	Use:               "remove <system>",
	Short:             "remove a context",
	Args:              cobra.ExactArgs(1),
	PersistentPreRunE: persistentPreRunE,
	RunE:              contextRemoveCmdFunc,
}

var contextUseCmd = &cobra.Command{
	Use:               "use <system>",
	Short:             "set a context as the current context",
	Args:              cobra.ExactArgs(1),
	PersistentPreRunE: persistentPreRunE,
	RunE:              contextUseCmdFunc,
}

func contextListCmdFunc(cmd *cobra.Command, args []string) error {
	tokens, err := storage.DefaultTokenStore.List(cobrautil.MustGetBool(cmd, "reveal-tokens"))
	if err != nil {
		return err
	}

	cfg, err := storage.DefaultConfigStore.Get()
	if err != nil {
		return err
	}

	var rows [][]string
	for _, token := range tokens {
		current := ""
		if token.System == cfg.CurrentToken {
			current = "   ✓   "
		}

		rows = append(rows, []string{
			current,
			token.System,
			token.Endpoint,
			stringz.Join("_", token.Prefix, token.Secret),
		})
	}

	printers.PrintTable(os.Stdout, []string{"current", "permissions system", "endpoint", "token"}, rows)

	return nil
}

func contextSetCmdFunc(cmd *cobra.Command, args []string) error {
	var name, token string
	err := stringz.Unpack(args, &name, &token)
	if err != nil {
		return err
	}
	endpoint := stringz.DefaultEmpty(cobrautil.MustGetString(cmd, "endpoint"), "grpc.authzed.com:443")

	err = storage.DefaultTokenStore.Put(name, endpoint, token)
	if err != nil {
		return err
	}

	return storage.SetCurrentToken(name, storage.DefaultConfigStore, storage.DefaultTokenStore)
}

func contextRemoveCmdFunc(cmd *cobra.Command, args []string) error {
	// If the token is what's currently being used, remove it from the config.
	cfg, err := storage.DefaultConfigStore.Get()
	if err != nil {
		return err
	}

	if cfg.CurrentToken == args[0] {
		cfg.CurrentToken = ""
	}
	err = storage.DefaultConfigStore.Put(cfg)
	if err != nil {
		return err
	}

	return storage.DefaultTokenStore.Delete(args[0])
}

func contextUseCmdFunc(cmd *cobra.Command, args []string) error {
	return storage.SetCurrentToken(args[0], storage.DefaultConfigStore, storage.DefaultTokenStore)
}
