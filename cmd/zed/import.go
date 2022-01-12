package main

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"strings"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/spicedb/pkg/tuple"
	"github.com/jzelinskie/cobrautil"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/decode"
	"github.com/authzed/zed/internal/storage"
)

func registerImportCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().Bool("schema", true, "import schema")
	importCmd.Flags().Bool("relationships", true, "import relationships")
}

var importCmd = &cobra.Command{
	Use:   "import <url>",
	Short: "import schema and relationships from a file or url",
	Example: `
	From a gist:
		zed import https://gist.github.com/ecordell/8e3b613a677e3c844742cf24421c08b6

	From a playground link:
		zed import https://play.authzed.com/s/iksdFvCtvnkR/schema

	From pastebin:
		zed import https://pastebin.com/8qU45rVK

	From a devtools instance:
		zed import https://localhost:8443/download

	From a local file:
		zed import file:///Users/zed/Downloads/authzed-x7izWU8_2Gw3.yaml

	Only schema: 
		zed import --relationships=false file:///Users/zed/Downloads/authzed-x7izWU8_2Gw3.yaml

	Only relationships:
		zed import --schema=false file:///Users/zed/Downloads/authzed-x7izWU8_2Gw3.yaml
`,
	Args: cobra.ExactArgs(1),
	RunE: cobrautil.CommandStack(LogCmdFunc, importCmdFunc),
}

func importCmdFunc(cmd *cobra.Command, args []string) error {
	configStore, secretStore := defaultStorage()
	token, err := storage.DefaultToken(
		cobrautil.MustGetString(cmd, "endpoint"),
		cobrautil.MustGetString(cmd, "token"),
		configStore,
		secretStore,
	)
	if err != nil {
		return err
	}
	log.Trace().Interface("token", token).Send()

	client, err := authzed.NewClient(token.Endpoint, dialOptsFromFlags(cmd, token.ApiToken)...)
	if err != nil {
		return err
	}

	u, err := url.Parse(args[0])
	if err != nil {
		return err
	}

	decoder, err := decode.DecoderForURL(u)
	if err != nil {
		return err
	}
	var p decode.SchemaRelationships
	if err := decoder(&p); err != nil {
		return err
	}

	if cobrautil.MustGetBool(cmd, "schema") {
		if err := importSchema(client, p.Schema); err != nil {
			return err
		}
	}

	if cobrautil.MustGetBool(cmd, "relationships") {
		if err := importRelationships(client, p.Relationships); err != nil {
			return err
		}
	}

	return err
}

func importSchema(client *authzed.Client, schema string) error {
	log.Info().Msg("importing schema")

	_, err := client.WriteSchema(context.Background(), &v1.WriteSchemaRequest{
		Schema: schema,
	})
	return err
}

func importRelationships(client *authzed.Client, relationships string) error {
	relationshipUpdates := make([]*v1.RelationshipUpdate, 0)
	scanner := bufio.NewScanner(strings.NewReader(relationships))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "//") {
			continue
		}
		rel := tuple.ParseRel(line)
		if rel == nil {
			return fmt.Errorf("failed to parse %s as relationship", line)
		}
		log.Trace().Str("line", line).Send()
		relationshipUpdates = append(relationshipUpdates, &v1.RelationshipUpdate{
			Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: rel,
		})
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	log.Info().Int("count", len(relationshipUpdates)).Msg("importing relationships")
	_, err := client.WriteRelationships(context.Background(), &v1.WriteRelationshipsRequest{
		Updates: relationshipUpdates,
	})
	return err
}
