package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/zed/internal/config"
	"github.com/jzelinskie/zed/internal/keychain"
	"github.com/jzelinskie/zed/internal/printers"
)

func setTokenCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("must provide only 2 arguments: name and token")
	}

	endpoint := cobrautil.MustGetString(cmd, "endpoint")
	if err := keychain.Put(
		endpoint,
		args[0],
		[]byte(args[1]),
	); err != nil {
		return err
	}

	fmt.Println("set token " + args[0])
	printers.PrintTable(
		os.Stdout,
		[]string{"name", "endpoint", "token"},
		[][]string{{args[0], endpoint, "<redacted>"}},
	)

	return nil
}

func getTokensCmdFunc(cmd *cobra.Command, args []string) error {
	names, err := keychain.List(cobrautil.MustGetString(cmd, "endpoint"))
	if err != nil {
		return err
	}

	var rows [][]string
	for _, name := range names {
		rows = append(rows, []string{name, "<redacted>"})
	}

	printers.PrintTable(os.Stdout, []string{"name", "token"}, rows)

	return nil
}

func getContextsCmdFunc(cmd *cobra.Command, args []string) error {
	cfg, err := config.Get()
	if err != nil {
		return err
	}

	var rows [][]string
	for _, context := range cfg.AvailableContexts {
		current := ""
		if context.Name == cfg.CurrentContext {
			current = "true"
		}

		rows = append(rows, []string{
			context.Name,
			context.Tenant,
			context.TokenName,
			current,
		})
	}

	printers.PrintTable(
		os.Stdout,
		[]string{"name", "tenant", "token name", "current"},
		rows,
	)

	return nil
}

func setContextCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("must provide only 3 arguments: name, tenant, and token name")
	}

	token, err := keychain.Get(cobrautil.MustGetString(cmd, "endpoint"), args[2])
	if err != nil {
		return err
	}
	if token == nil {
		return fmt.Errorf("no token in keychain with name: %s", args[2])
	}

	cfg, err := config.Get()
	if err != nil {
		return err
	}

	cfg.WithContext(config.Context{
		Name:      args[0],
		Tenant:    args[1],
		TokenName: args[2],
	})

	if err := config.Put(cfg); err != nil {
		return err
	}

	printers.PrintTable(
		os.Stdout,
		[]string{"name", "tenant", "token name", "current"},
		[][]string{{args[0], args[1], args[2]}},
	)

	return nil
}

func useContextCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("must provide only 1 argument: name")
	}

	cfg, err := config.Get()
	if err != nil {
		return err
	}

	for _, context := range cfg.AvailableContexts {
		if context.Name == args[0] {
			cfg.CurrentContext = context.Name
			if err := config.Put(cfg); err != nil {
				return err
			}

			printers.PrintTable(
				os.Stdout,
				[]string{"name", "tenant", "token name", "current"},
				[][]string{{context.Name, context.Tenant, context.TokenName, "true"}},
			)

			return nil
		}
	}

	return fmt.Errorf("could not find available context: %s", args[0])
}
