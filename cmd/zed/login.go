package main

import (
	"context"
	"fmt"
	"os"

	v0 "github.com/authzed/authzed-go/proto/authzed/api/v0"
	authzedv0 "github.com/authzed/authzed-go/v0"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/authzed/zed/internal/storage"
)

func loginCmdFunc(cmd *cobra.Command, args []string) error {
	var name string
	err := stringz.Unpack(args, &name)
	if err != nil {
		return err
	}
	endpoint := stringz.DefaultEmpty(cobrautil.MustGetString(cmd, "endpoint"), "grpc.authzed.com:443")

	fmt.Printf("token: ")
	b, err := terminal.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	fmt.Println()
	token := string(b)

	client, err := authzedv0.NewClient(endpoint, dialOptsFromFlags(cmd, token)...)
	if err != nil {
		return err
	}
	request := &v0.ReadConfigRequest{
		Namespace: name,
	}

	log.Trace().Interface("request", request).Msg("requesting namespace read")
	_, err = client.ReadConfig(context.Background(), request)
	if err != nil {
		return errors.WithMessagef(err, "couldn't connect to namespace %q at %q", name, endpoint)
	}
	fmt.Println("login successful")
	err = storage.DefaultTokenStore.Put(name, endpoint, token)
	if err != nil {
		return err
	}

	return storage.SetCurrentToken(name, storage.DefaultConfigStore, storage.DefaultTokenStore)
}
