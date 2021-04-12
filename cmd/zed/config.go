package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jzelinskie/zed/internal/config"
	"github.com/jzelinskie/zed/internal/keychain"
)

func setTokenCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("must provide only 2 arguments: name and token")
	}

	return keychain.Put("authzed.com", args[0], []byte(args[1]))
}

func listTokensCmdFunc(cmd *cobra.Command, args []string) error {
	names, err := keychain.List("authzed.com")
	if err != nil {
		return err
	}

	for _, name := range names {
		fmt.Println(name)
	}

	return nil
}

func listContextsCmdFunc(cmd *cobra.Command, args []string) error {
	cfg, err := config.Get()
	if err != nil {
		return err
	}

	for _, context := range cfg.AvailableContexts {
		if context.Name == cfg.CurrentContext {
			fmt.Println(context.String() + " (current)")
		} else {
			fmt.Println(context)
		}
	}

	return nil
}

func setContextCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("must provide only 3 arguments: name, tenant, and token name")
	}

	token, err := keychain.Get("authzed.com", args[2])
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

	return config.Put(cfg)
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
			return config.Put(cfg)
		}
	}

	return fmt.Errorf("could not find available context: %s", args[0])
}
