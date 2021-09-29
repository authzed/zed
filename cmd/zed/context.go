package main

import (
	"fmt"
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
	Use:               "set <name> <endpoint> <api-token>",
	Short:             "create or overwrite a context",
	Args:              cobra.ExactArgs(3),
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
	Args:              cobra.MaximumNArgs(1),
	PersistentPreRunE: persistentPreRunE,
	RunE:              contextUseCmdFunc,
}

func contextListCmdFunc(cmd *cobra.Command, args []string) error {
	cfgStore, secretStore := defaultStorage()
	secrets, err := secretStore.Get()
	if err != nil {
		return err
	}

	cfg, err := cfgStore.Get()
	if err != nil {
		return err
	}

	var rows [][]string
	for _, token := range secrets.Tokens {
		current := ""
		if token.Name == cfg.CurrentToken {
			current = "   ✓   "
		}
		secret := token.ApiToken
		if !cobrautil.MustGetBool(cmd, "reveal-tokens") {
			prefix, _ := token.SplitApiToken()
			secret = stringz.Join("_", prefix, "<redacted>")
		}

		rows = append(rows, []string{
			current,
			token.Name,
			token.Endpoint,
			secret,
		})
	}

	printers.PrintTable(os.Stdout, []string{"current", "name", "endpoint", "token"}, rows)

	return nil
}

func contextSetCmdFunc(cmd *cobra.Command, args []string) error {
	var name, endpoint, apiToken string
	err := stringz.Unpack(args, &name, &endpoint, &apiToken)
	if err != nil {
		return err
	}

	cfgStore, secretStore := defaultStorage()
	err = storage.PutToken(storage.Token{
		Name:     name,
		Endpoint: stringz.DefaultEmpty(endpoint, "grpc.authzed.com:443"),
		ApiToken: apiToken,
	}, secretStore)
	if err != nil {
		return err
	}

	return storage.SetCurrentToken(name, cfgStore, secretStore)
}

func contextRemoveCmdFunc(cmd *cobra.Command, args []string) error {
	// If the token is what's currently being used, remove it from the config.
	cfgStore, secretStore := defaultStorage()
	cfg, err := cfgStore.Get()
	if err != nil {
		return err
	}

	if cfg.CurrentToken == args[0] {
		cfg.CurrentToken = ""
	}

	err = cfgStore.Put(cfg)
	if err != nil {
		return err
	}

	return storage.RemoveToken(args[0], secretStore)
}

func contextUseCmdFunc(cmd *cobra.Command, args []string) error {
	cfgStore, secretStore := defaultStorage()
	switch len(args) {
	case 0:
		cfg, err := cfgStore.Get()
		if err != nil {
			return err
		}
		fmt.Println(cfg.CurrentToken)
	case 1:
		return storage.SetCurrentToken(args[0], cfgStore, secretStore)
	default:
		panic("cobra command did not enforce valid number of args")
	}

	return nil
}
