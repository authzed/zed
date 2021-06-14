package main

import (
	"os"

	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/printers"
	"github.com/authzed/zed/internal/storage"
)

var tokenCmd = &cobra.Command{
	Use:   "token <subcommand>",
	Short: "manage the API Tokens stored on your machine",
}

var tokenListCmd = &cobra.Command{
	Use:               "list",
	Short:             "list all the API Tokens",
	Args:              cobra.ExactArgs(0),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              tokenListCmdFunc,
}

var tokenSaveCmd = &cobra.Command{
	Use:               "save <system> <token>",
	Short:             "save an API Token",
	Args:              cobra.ExactArgs(2),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              tokenSaveCmdFunc,
}

var tokenDeleteCmd = &cobra.Command{
	Use:               "delete <system>",
	Short:             "delete an API Token",
	Args:              cobra.ExactArgs(1),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              tokenDeleteCmdFunc,
}

var tokenUseCmd = &cobra.Command{
	Use:               "use <system>",
	Short:             "use an API Token",
	Args:              cobra.ExactArgs(1),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              tokenUseCmdFunc,
}

func tokenListCmdFunc(cmd *cobra.Command, args []string) error {
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
		using := ""
		if token.Name == cfg.CurrentToken {
			using = "true"
		}

		rows = append(rows, []string{
			using,
			token.Name,
			token.Endpoint,
			stringz.Join("_", token.Prefix, token.Secret),
		})
	}

	printers.PrintTable(os.Stdout, []string{"using", "name", "endpoint", "token"}, rows)

	return nil
}

func tokenSaveCmdFunc(cmd *cobra.Command, args []string) error {
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

func tokenDeleteCmdFunc(cmd *cobra.Command, args []string) error {
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

func tokenUseCmdFunc(cmd *cobra.Command, args []string) error {
	return storage.SetCurrentToken(args[0], storage.DefaultConfigStore, storage.DefaultTokenStore)
}
