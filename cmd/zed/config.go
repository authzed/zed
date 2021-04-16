package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/printers"
	"github.com/authzed/zed/internal/storage"
)

func setTokenCmdFunc(cmd *cobra.Command, args []string) error {
	var name, token string
	stringz.Unpack(args, &name, &token)
	endpoint := stringz.DefaultEmpty(cobrautil.MustGetString(cmd, "endpoint"), "grpc.authzed.com:443")

	if err := tokenStore.Put(storage.Token{
		Name:     name,
		Endpoint: endpoint,
		Token:    token,
	}); err != nil {
		return err
	}

	printers.PrintTable(
		os.Stdout,
		[]string{"name", "endpoint", "token"},
		[][]string{{name, endpoint, "<redacted>"}},
	)

	return nil
}

func renameTokenCmdFunc(cmd *cobra.Command, args []string) error {
	var oldName, newName string
	stringz.Unpack(args, &oldName, &newName)

	if oldName == newName {
		return nil
	}

	token, err := tokenStore.Get(oldName, false)
	if err != nil {
		return err
	}

	token.Name = newName
	if err := tokenStore.Put(token); err != nil {
		return err
	}

	cfg, err := contextConfigStore.Get()
	if err != nil {
		return err
	}

	for i, context := range cfg.AvailableContexts {
		if context.TokenName == oldName {
			cfg.AvailableContexts[i].TokenName = newName
		}
	}

	if err := contextConfigStore.Put(cfg); err != nil {
		return err
	}

	if err := tokenStore.Delete(oldName); err != nil {
		return err
	}

	printers.PrintTable(
		os.Stdout,
		[]string{"name", "endpoint", "token"},
		[][]string{{token.Name, token.Endpoint, "<redacted>"}},
	)

	return nil
}

func deleteTokenCmdFunc(cmd *cobra.Command, args []string) error {
	tokenName := args[0]

	cfg, err := contextConfigStore.Get()
	if err != nil {
		return err
	}

	var filtered []storage.Context
	for _, context := range cfg.AvailableContexts {
		if context.TokenName == tokenName {
			fmt.Println("deleted context: " + context.Name)
			continue
		}
		filtered = append(filtered, context)
	}

	if len(cfg.AvailableContexts) != len(filtered) {
		cfg.AvailableContexts = filtered
		if err := contextConfigStore.Put(cfg); err != nil {
			return err
		}
	}

	if err := tokenStore.Delete(tokenName); err != nil {
		return err
	}

	fmt.Println("deleted token: " + tokenName)

	return nil
}

func getTokensCmdFunc(cmd *cobra.Command, args []string) error {
	tokens, err := tokenStore.List(!cobrautil.MustGetBool(cmd, "reveal-tokens"))
	if err != nil {
		return err
	}

	var rows [][]string
	for _, token := range tokens {
		rows = append(rows, []string{
			token.Name,
			token.Endpoint,
			token.Token,
		})
	}

	printers.PrintTable(os.Stdout, []string{"name", "endpoint", "token"}, rows)

	return nil
}

func getContextsCmdFunc(cmd *cobra.Command, args []string) error {
	cfg, err := contextConfigStore.Get()
	if err != nil {
		return err
	}

	var rows [][]string
	for _, context := range cfg.AvailableContexts {
		current := ""
		if context.Name == cfg.CurrentContext {
			current = "true"
		}

		token, err := tokenStore.Get(context.TokenName, true)
		if err != nil {
			return err
		}

		rows = append(rows, []string{
			context.Name,
			context.Tenant,
			context.TokenName,
			token.Endpoint,
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
	var oldName, newName string
	stringz.Unpack(args, &oldName, &newName)

	if oldName == newName {
		return nil
	}

	cfg, err := contextConfigStore.Get()
	if err != nil {
		return err
	}

	var foundContext storage.Context
	for i, context := range cfg.AvailableContexts {
		if context.Name == oldName {
			cfg.AvailableContexts[i].Name = newName
			foundContext = cfg.AvailableContexts[i]
			break
		}
	}
	if foundContext.Name == "" {
		return fmt.Errorf("could not find context: " + oldName)
	}

	if cfg.CurrentContext == oldName {
		cfg.CurrentContext = newName
	}

	if err := contextConfigStore.Put(cfg); err != nil {
		return err
	}

	token, err := tokenStore.Get(foundContext.TokenName, true)
	if err != nil {
		return err
	}

	printers.PrintTable(
		os.Stdout,
		[]string{"name", "tenant", "token name", "endpoint", "current"},
		[][]string{{
			newName,
			foundContext.Tenant,
			foundContext.TokenName,
			token.Endpoint,
			strconv.FormatBool(cfg.CurrentContext == newName),
		}},
	)

	return nil
}

func deleteContextCmdFunc(cmd *cobra.Command, args []string) error {
	cfg, err := contextConfigStore.Get()
	if err != nil {
		return err
	}

	var filtered []storage.Context
	for _, context := range cfg.AvailableContexts {
		if context.TokenName != args[0] {
			filtered = append(filtered, context)
		}
	}

	if len(cfg.AvailableContexts) != len(filtered) {
		cfg.AvailableContexts = filtered
		if err := contextConfigStore.Put(cfg); err != nil {
			return err
		}

		fmt.Println("deleted context: " + args[0])
		return nil
	}

	return fmt.Errorf("could not find context: " + args[0])
}

func setContextCmdFunc(cmd *cobra.Command, args []string) error {
	var newContext storage.Context
	stringz.Unpack(args, &newContext.Name, &newContext.Tenant, &newContext.TokenName)

	token, err := tokenStore.Get(newContext.TokenName, true)
	if err != nil {
		return err
	}

	cfg, err := contextConfigStore.Get()
	if err != nil {
		return err
	}

	cfg.AppendAvailableContext(newContext)

	if len(cfg.AvailableContexts) == 1 {
		cfg.CurrentContext = newContext.Name
	}

	if err := contextConfigStore.Put(cfg); err != nil {
		return err
	}

	printers.PrintTable(
		os.Stdout,
		[]string{"name", "tenant", "token name", "endpoint", "current"},
		[][]string{{
			newContext.Name,
			newContext.Tenant,
			newContext.TokenName,
			token.Endpoint,
			strconv.FormatBool(cfg.CurrentContext == newContext.Name),
		}},
	)

	return nil
}

func useContextCmdFunc(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := contextConfigStore.Get()
	if err != nil {
		return err
	}

	for _, context := range cfg.AvailableContexts {
		if context.Name == name {
			cfg.CurrentContext = context.Name
			if err := contextConfigStore.Put(cfg); err != nil {
				return err
			}

			token, err := tokenStore.Get(context.TokenName, true)
			if err != nil {
				return err
			}

			printers.PrintTable(
				os.Stdout,
				[]string{"name", "tenant", "token name", "endpoint", "current"},
				[][]string{{context.Name, context.Tenant, context.TokenName, token.Endpoint, "true"}},
			)

			return nil
		}
	}

	return fmt.Errorf("could not find available context: %s", args[0])
}
