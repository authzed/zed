package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/spicedb/pkg/schemadsl/compiler"
	"github.com/authzed/spicedb/pkg/schemadsl/generator"
	"github.com/authzed/spicedb/pkg/schemadsl/input"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/commands"
	"github.com/authzed/zed/internal/console"
	"github.com/authzed/zed/internal/storage"
)

func registerAdditionalSchemaCmds(schemaCmd *cobra.Command) {
	schemaCmd.AddCommand(schemaCopyCmd)
	schemaCopyCmd.Flags().Bool("json", false, "output as JSON")
	schemaCopyCmd.Flags().String("schema-definition-prefix", "", "prefix to add to the schema's definition(s) before writing")

	schemaCmd.AddCommand(schemaWriteCmd)
	schemaWriteCmd.Flags().Bool("json", false, "output as JSON")
	schemaWriteCmd.Flags().String("schema-definition-prefix", "", "prefix to add to the schema's definition(s) before writing")
}

var schemaWriteCmd = &cobra.Command{
	Use:   "write <file?>",
	Args:  cobra.MaximumNArgs(1),
	Short: "write a Schema file (or stdin) to the current Permissions System",
	RunE:  schemaWriteCmdFunc,
}

var schemaCopyCmd = &cobra.Command{
	Use:   "copy <src context> <dest context>",
	Args:  cobra.ExactArgs(2),
	Short: "copy a Schema from one context into another",
	RunE:  schemaCopyCmdFunc,
}

// TODO(jschorr): support this in the client package
func clientForContext(cmd *cobra.Command, contextName string, secretStore storage.SecretStore) (*authzed.Client, error) {
	token, err := storage.GetToken(contextName, secretStore)
	if err != nil {
		return nil, err
	}
	log.Trace().Interface("token", token).Send()

	dialOpts, err := client.DialOptsFromFlags(cmd, token)
	if err != nil {
		return nil, err
	}
	return authzed.NewClient(token.Endpoint, dialOpts...)
}

func schemaCopyCmdFunc(cmd *cobra.Command, args []string) error {
	_, secretStore := client.DefaultStorage()
	srcClient, err := clientForContext(cmd, args[0], secretStore)
	if err != nil {
		return err
	}
	destClient, err := clientForContext(cmd, args[1], secretStore)
	if err != nil {
		return err
	}

	readRequest := &v1.ReadSchemaRequest{}
	log.Trace().Interface("request", readRequest).Msg("requesting schema read")

	readResp, err := srcClient.ReadSchema(context.Background(), readRequest)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to read schema")
	}
	log.Trace().Interface("response", readResp).Msg("read schema")

	prefix, err := determinePrefixForSchema(cobrautil.MustGetString(cmd, "schema-definition-prefix"), nil, &readResp.SchemaText)
	if err != nil {
		return err
	}

	schemaText, err := rewriteSchema(readResp.SchemaText, prefix)
	if err != nil {
		return err
	}

	writeRequest := &v1.WriteSchemaRequest{Schema: schemaText}
	log.Trace().Interface("request", writeRequest).Msg("writing schema")

	resp, err := destClient.WriteSchema(context.Background(), writeRequest)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to write schema")
	}
	log.Trace().Interface("response", resp).Msg("wrote schema")

	if cobrautil.MustGetBool(cmd, "json") {
		prettyProto, err := commands.PrettyProto(resp)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to convert schema to JSON")
		}

		console.Println(string(prettyProto))
		return nil
	}

	return nil
}

func schemaWriteCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) == 0 && term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("must provide file path or contents via stdin")
	}

	client, err := client.NewClient(cmd)
	if err != nil {
		return err
	}
	var schemaBytes []byte
	switch len(args) {
	case 1:
		schemaBytes, err = os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("failed to read schema file: %w", err)
		}
		log.Trace().Str("schema", string(schemaBytes)).Str("file", args[0]).Msg("read schema from file")
	case 0:
		schemaBytes, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read schema file: %w", err)
		}
		log.Trace().Str("schema", string(schemaBytes)).Msg("read schema from stdin")
	default:
		panic("schemaWriteCmdFunc called with incorrect number of arguments")
	}

	if len(schemaBytes) == 0 {
		return errors.New("attempted to write empty schema")
	}

	prefix, err := determinePrefixForSchema(cobrautil.MustGetString(cmd, "schema-definition-prefix"), client, nil)
	if err != nil {
		return err
	}

	schemaText, err := rewriteSchema(string(schemaBytes), prefix)
	if err != nil {
		return err
	}

	request := &v1.WriteSchemaRequest{Schema: schemaText}
	log.Trace().Interface("request", request).Msg("writing schema")

	resp, err := client.WriteSchema(context.Background(), request)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to write schema")
	}
	log.Trace().Interface("response", resp).Msg("wrote schema")

	if cobrautil.MustGetBool(cmd, "json") {
		prettyProto, err := commands.PrettyProto(resp)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to convert schema to JSON")
		}

		console.Println(string(prettyProto))
		return nil
	}

	return nil
}

// rewriteSchema rewrites the given existing schema to include the specified prefix on all definitions.
func rewriteSchema(existingSchemaText string, definitionPrefix string) (string, error) {
	if definitionPrefix == "" {
		return existingSchemaText, nil
	}

	compiled, err := compiler.Compile(compiler.InputSchema{
		Source: input.Source("schema"), SchemaString: existingSchemaText,
	}, &definitionPrefix)
	if err != nil {
		return "", err
	}

	generated, _, err := generator.GenerateSchema(compiled.OrderedDefinitions)
	return generated, err
}

// determinePrefixForSchema determines the prefix to be applied to a schema that will be written.
//
// If specifiedPrefix is non-empty, it is returned immediately.
// If existingSchema is non-nil, it is parsed for the prefix.
// Otherwise, the client is used to retrieve the existing schema (if any), and the prefix is retrieved from there.
func determinePrefixForSchema(specifiedPrefix string, client client.Client, existingSchema *string) (string, error) {
	if specifiedPrefix != "" {
		return specifiedPrefix, nil
	}

	var schemaText string
	if existingSchema != nil {
		schemaText = *existingSchema
	} else {
		readSchemaText, err := commands.ReadSchema(client)
		if err != nil {
			return "", nil
		}
		schemaText = readSchemaText
	}

	// If there is no schema found, return the empty string.
	if schemaText == "" {
		return "", nil
	}

	// Otherwise, compile the schema and grab the prefixes of the namespaces defined.
	empty := ""
	found, err := compiler.Compile(compiler.InputSchema{
		Source: input.Source("schema"), SchemaString: schemaText,
	}, &empty)
	if err != nil {
		return "", err
	}

	foundPrefixes := make([]string, 0, len(found.OrderedDefinitions))
	for _, def := range found.OrderedDefinitions {
		if strings.Contains(def.GetName(), "/") {
			parts := strings.Split(def.GetName(), "/")
			foundPrefixes = append(foundPrefixes, parts[0])
		} else {
			foundPrefixes = append(foundPrefixes, "")
		}
	}

	prefixes := stringz.Dedup(foundPrefixes)
	if len(prefixes) == 1 {
		prefix := prefixes[0]
		log.Debug().Str("prefix", prefix).Msg("found schema definition prefix")
		return prefix, nil
	}

	return "", nil
}
