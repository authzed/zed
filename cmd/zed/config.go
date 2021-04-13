package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/dustin/go-humanize"
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
		"zed token",
		args[1],
	); err != nil {
		return err
	}

	printers.PrintTable(
		os.Stdout,
		[]string{"name", "endpoint", "token", "modified"},
		[][]string{{args[0], endpoint, "<redacted>", "now"}},
	)

	return nil
}

func renameTokenCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("must provide only 2 arguments: old name and new name")
	}

	if args[0] == args[1] {
		return nil
	}

	item, err := keychain.Get(args[0], "zed token", true)
	if err != nil {
		return err
	}

	if err := keychain.Put(item.Service, args[1], "zed token", string(item.Data)); err != nil {
		return err
	}

	if err := keychain.Delete(args[0], "zed token"); err != nil {
		return err
	}

	cfg, err := config.Get()
	if err != nil {
		return err
	}

	for i, context := range cfg.AvailableContexts {
		if context.TokenName == args[0] {
			cfg.AvailableContexts[i].TokenName = args[1]
		}
	}

	if err := config.Put(cfg); err != nil {
		return err
	}

	printers.PrintTable(
		os.Stdout,
		[]string{"name", "endpoint", "token", "modified"},
		[][]string{{args[1], item.Service, "<redacted>", "now"}},
	)

	return nil
}

func deleteTokenCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("must provide only 1 argument: name")
	}

	cfg, err := config.Get()
	if err != nil {
		return err
	}

	var filtered []config.Context
	for _, context := range cfg.AvailableContexts {
		if context.TokenName == args[0] {
			fmt.Println("deleted context: " + context.Name)
			continue
		}
		filtered = append(filtered, context)
	}

	if len(cfg.AvailableContexts) != len(filtered) {
		cfg.AvailableContexts = filtered
		if err := config.Put(cfg); err != nil {
			return err
		}
	}

	if err := keychain.Delete(args[0], "zed token"); err != nil {
		return err
	}

	fmt.Println("deleted token: " + args[0])

	return nil
}

func getTokensCmdFunc(cmd *cobra.Command, args []string) error {
	items, err := keychain.ListByLabel("zed token")
	if err != nil {
		return err
	}

	var rows [][]string
	for _, item := range items {
		token := "<redacted>"
		if cobrautil.MustGetBool(cmd, "reveal-tokens") {
			item, err := keychain.Get(item.Account, "zed token", true)
			if err != nil {
				return err
			}
			token = string(item.Data)
		}

		rows = append(rows, []string{
			item.Account,
			item.Service,
			token,
			humanize.Time(item.ModificationDate),
		})
	}

	printers.PrintTable(os.Stdout, []string{"name", "endpoint", "token", "modified"}, rows)

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

		item, err := keychain.Get(context.TokenName, "zed token", false)
		if err != nil {
			return err
		}

		if item == nil {
			continue
		}

		rows = append(rows, []string{
			context.Name,
			context.Tenant,
			context.TokenName,
			item.Service,
			current,
		})
	}

	printers.PrintTable(
		os.Stdout,
		[]string{"name", "tenant", "token name", "endpoint", "current"},
		rows,
	)

	return nil
}

func renameContextCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("must provide only 2 arguments: old name and new name")
	}
	if args[0] == args[1] {
		return nil
	}

	cfg, err := config.Get()
	if err != nil {
		return err
	}

	var found *config.Context
	for _, context := range cfg.AvailableContexts {
		if context.Name == args[0] {
			found = &context
			break
		}
	}

	if found == nil {
		return fmt.Errorf("could not find context: " + args[0])
	}

	if cfg.CurrentContext == args[0] {
		cfg.CurrentContext = args[1]
	}

	if err := config.Put(cfg); err != nil {
		return err
	}

	token, err := keychain.Get(found.TokenName, "zed token", false)
	if err != nil {
		return err
	}
	strconv.FormatBool(cfg.CurrentContext == args[1])

	printers.PrintTable(
		os.Stdout,
		[]string{"name", "tenant", "token name", "endpoint", "current"},
		[][]string{{
			args[1],
			found.Tenant,
			found.TokenName,
			token.Service,
			strconv.FormatBool(cfg.CurrentContext == args[1]),
		}},
	)

	return nil
}

func deleteContextCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("must provide only 1 argument: name")
	}

	cfg, err := config.Get()
	if err != nil {
		return err
	}

	var filtered []config.Context
	for _, context := range cfg.AvailableContexts {
		if context.TokenName != args[0] {
			filtered = append(filtered, context)
		}
	}

	if len(cfg.AvailableContexts) != len(filtered) {
		cfg.AvailableContexts = filtered
		if err := config.Put(cfg); err != nil {
			return err
		}

		fmt.Println("deleted context: " + args[0])
		return nil
	}

	return fmt.Errorf("could not find context: " + args[0])
}

func setContextCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("must provide only 3 arguments: name, tenant, and token name")
	}

	token, err := keychain.Get(args[2], "zed token", false)
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
		[]string{"name", "tenant", "token name", "endpoint", "current"},
		[][]string{{args[0], args[1], args[2], token.Service, ""}},
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

			token, err := keychain.Get(context.TokenName, "zed token", false)
			if err != nil {
				return err
			}

			printers.PrintTable(
				os.Stdout,
				[]string{"name", "tenant", "token name", "endpoint", "current"},
				[][]string{{context.Name, context.Tenant, context.TokenName, token.Service, "true"}},
			)

			return nil
		}
	}

	return fmt.Errorf("could not find available context: %s", args[0])
}
