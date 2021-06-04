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
	Short: "manage the API tokens stored in your keychain",
}

var tokenListCmd = &cobra.Command{
	Use:               "list",
	Short:             "list all the API Tokens from your keychain",
	Args:              cobra.ExactArgs(0),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              tokenListCmdFunc,
}

var tokenSaveCmd = &cobra.Command{
	Use:               "save <system> <token>",
	Short:             "save an API Token to your keychain",
	Args:              cobra.ExactArgs(2),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              tokenSaveCmdFunc,
}

var tokenDeleteCmd = &cobra.Command{
	Use:               "delete <system>",
	Short:             "delete an API Token from your keychain",
	Args:              cobra.ExactArgs(1),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              tokenDeleteCmdFunc,
}

var tokenUseCmd = &cobra.Command{
	Use:               "use <system>",
	Short:             "use an API Token for future commands",
	Args:              cobra.ExactArgs(1),
	PersistentPreRunE: cobrautil.SyncViperPreRunE("ZED"),
	RunE:              tokenUseCmdFunc,
}

func tokenListCmdFunc(cmd *cobra.Command, args []string) error {
	tokens, err := tokenStore.List(!cobrautil.MustGetBool(cmd, "reveal-tokens"))
	if err != nil {
		return err
	}

	var rows [][]string
	for _, token := range tokens {
		rows = append(rows, []string{
			token.Name,
			token.Endpoint,
			token.ApiToken,
		})
	}

	printers.PrintTable(os.Stdout, []string{"name", "endpoint", "token"}, rows)

	return nil
}

func tokenSaveCmdFunc(cmd *cobra.Command, args []string) error {
	var name, token string
	err := stringz.Unpack(args, &name, &token)
	if err != nil {
		return err
	}
	endpoint := stringz.DefaultEmpty(cobrautil.MustGetString(cmd, "endpoint"), "grpc.authzed.com:443")

	if err := tokenStore.Put(storage.Token{
		Name:     name,
		Endpoint: endpoint,
		ApiToken: token,
	}); err != nil {
		return err
	}

	printers.PrintTable(
		os.Stdout,
		[]string{"system", "endpoint", "token"},
		[][]string{{name, endpoint, "<redacted>"}},
	)

	return storage.SetCurrentToken(name, configStore, tokenStore)
}

func tokenDeleteCmdFunc(cmd *cobra.Command, args []string) error {
	// If the token is what's currently being used, remove it from the config.
	cfg, err := configStore.Get()
	if err != nil {
		return err
	}

	if cfg.CurrentToken == args[0] {
		cfg.CurrentToken = ""
	}
	err = configStore.Put(cfg)
	if err != nil {
		return err
	}

	return tokenStore.Delete(args[0])
}

func tokenUseCmdFunc(cmd *cobra.Command, args []string) error {
	return storage.SetCurrentToken(args[0], configStore, tokenStore)
}
