package cmd

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/jzelinskie/cobrautil/v2"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/tuple"
	"github.com/authzed/spicedb/pkg/validationfile"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/commands"
	"github.com/authzed/zed/internal/decode"
	"github.com/authzed/zed/internal/grpcutil"
)

func registerImportCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().Int("batch-size", 1000, "import batch size")
	importCmd.Flags().Int("workers", 1, "number of concurrent batching workers")
	importCmd.Flags().Bool("schema", true, "import schema")
	importCmd.Flags().Bool("relationships", true, "import relationships")
	importCmd.Flags().String("schema-definition-prefix", "", "prefix to add to the schema's definition(s) before importing")
}

var importCmd = &cobra.Command{
	Use:   "import <url>",
	Short: "Imports schema and relationships from a file or url",
	Example: `
	From a gist:
		zed import https://gist.github.com/ecordell/8e3b613a677e3c844742cf24421c08b6

	From a playground link:
		zed import https://play.authzed.com/s/iksdFvCtvnkR/schema

	From pastebin:
		zed import https://pastebin.com/8qU45rVK

	From a devtools instance:
		zed import https://localhost:8443/download

	From a local file (with prefix):
		zed import file:///Users/zed/Downloads/authzed-x7izWU8_2Gw3.yaml

	From a local file (no prefix):
		zed import authzed-x7izWU8_2Gw3.yaml

	Only schema:
		zed import --relationships=false file:///Users/zed/Downloads/authzed-x7izWU8_2Gw3.yaml

	Only relationships:
		zed import --schema=false file:///Users/zed/Downloads/authzed-x7izWU8_2Gw3.yaml

	With schema definition prefix:
		zed import --schema-definition-prefix=mypermsystem file:///Users/zed/Downloads/authzed-x7izWU8_2Gw3.yaml
`,
	Args: commands.ValidationWrapper(cobra.ExactArgs(1)),
	RunE: importCmdFunc,
}

func importCmdFunc(cmd *cobra.Command, args []string) error {
	client, err := client.NewClient(cmd)
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
	var p validationfile.ValidationFile
	if _, _, err := decoder(&p); err != nil {
		return err
	}

	prefix, err := determinePrefixForSchema(cmd.Context(), cobrautil.MustGetString(cmd, "schema-definition-prefix"), client, nil)
	if err != nil {
		return err
	}

	if cobrautil.MustGetBool(cmd, "schema") {
		if err := importSchema(cmd.Context(), client, p.Schema.Schema, prefix); err != nil {
			return err
		}
	}

	if cobrautil.MustGetBool(cmd, "relationships") {
		batchSize := cobrautil.MustGetInt(cmd, "batch-size")
		workers := cobrautil.MustGetInt(cmd, "workers")
		if err := importRelationships(cmd.Context(), client, p.Relationships.RelationshipsString, prefix, batchSize, workers); err != nil {
			return err
		}
	}

	return err
}

func importSchema(ctx context.Context, client client.Client, schema string, definitionPrefix string) error {
	log.Info().Msg("importing schema")

	// Recompile the schema with the specified prefix.
	schemaText, err := rewriteSchema(schema, definitionPrefix)
	if err != nil {
		return err
	}

	// Write the recompiled and regenerated schema.
	request := &v1.WriteSchemaRequest{Schema: schemaText}
	log.Trace().Interface("request", request).Str("schema", schemaText).Msg("writing schema")

	if _, err := client.WriteSchema(ctx, request); err != nil {
		return err
	}

	return nil
}

func importRelationships(ctx context.Context, client client.Client, relationships string, definitionPrefix string, batchSize int, workers int) error {
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
		rel, err := tuple.ParseV1Rel(line)
		if err != nil {
			return fmt.Errorf("failed to parse %s as relationship", line)
		}
		log.Trace().Str("line", line).Send()

		// Rewrite the prefix on the references, if any.
		if len(definitionPrefix) > 0 {
			rel.Resource.ObjectType = fmt.Sprintf("%s/%s", definitionPrefix, rel.Resource.ObjectType)
			rel.Subject.Object.ObjectType = fmt.Sprintf("%s/%s", definitionPrefix, rel.Subject.Object.ObjectType)
		}

		relationshipUpdates = append(relationshipUpdates, &v1.RelationshipUpdate{
			Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: rel,
		})
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	log.Info().
		Int("batch_size", batchSize).
		Int("workers", workers).
		Int("count", len(relationshipUpdates)).
		Msg("importing relationships")

	err := grpcutil.ConcurrentBatch(ctx, len(relationshipUpdates), batchSize, workers, func(ctx context.Context, no int, start int, end int) error {
		request := &v1.WriteRelationshipsRequest{Updates: relationshipUpdates[start:end]}
		_, err := client.WriteRelationships(ctx, request)
		if err != nil {
			return err
		}

		log.Info().
			Int("batch_no", no).
			Int("count", len(relationshipUpdates[start:end])).
			Msg("imported relationships")
		return nil
	})
	return err
}
