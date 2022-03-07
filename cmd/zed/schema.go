package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/TylerBrock/colorjson"
	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/authzed/spicedb/pkg/schemadsl/compiler"
	"github.com/authzed/spicedb/pkg/schemadsl/generator"
	"github.com/authzed/spicedb/pkg/schemadsl/input"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/authzed/zed/internal/storage"
)

func registerSchemaCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(schemaCmd)

	schemaCmd.AddCommand(schemaReadCmd)
	schemaReadCmd.Flags().Bool("json", false, "output as JSON")

	schemaCmd.AddCommand(schemaWriteCmd)
	schemaWriteCmd.Flags().Bool("json", false, "output as JSON")
	schemaWriteCmd.Flags().String("schema-definition-prefix", "", "prefix to add to the schema's definition(s) before writing")

	schemaCmd.AddCommand(schemaCopyCmd)
	schemaCopyCmd.Flags().Bool("json", false, "output as JSON")
	schemaCopyCmd.Flags().String("schema-definition-prefix", "", "prefix to add to the schema's definition(s) before writing")
}

var (
	schemaCmd = &cobra.Command{
		Use:   "schema <subcommand>",
		Short: "read and write to a Schema for a Permissions System",
	}

	schemaReadCmd = &cobra.Command{
		Use:   "read",
		Args:  cobra.ExactArgs(0),
		Short: "read the Schema of current Permissions System",
		RunE:  cobrautil.CommandStack(LogCmdFunc, schemaReadCmdFunc),
	}

	schemaWriteCmd = &cobra.Command{
		Use:   "write <file?>",
		Args:  cobra.MaximumNArgs(1),
		Short: "write a Schema file (or stdin) to the current Permissions System",
		RunE:  cobrautil.CommandStack(LogCmdFunc, schemaWriteCmdFunc),
	}

	schemaCopyCmd = &cobra.Command{
		Use:   "copy <src context> <dest context>",
		Args:  cobra.ExactArgs(2),
		Short: "copy a Schema from one context into another",
		RunE:  cobrautil.CommandStack(LogCmdFunc, schemaCopyCmdFunc),
	}
)

func schemaReadCmdFunc(cmd *cobra.Command, args []string) error {
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

	client, err := authzed.NewClient(token.Endpoint, dialOptsFromFlags(cmd, token.APIToken)...)
	if err != nil {
		return err
	}

	request := &v1.ReadSchemaRequest{}
	log.Trace().Interface("request", request).Msg("requesting schema read")

	resp, err := client.ReadSchema(context.Background(), request)
	if err != nil {
		return err
	}

	if cobrautil.MustGetBool(cmd, "json") {
		prettyProto, err := prettyProto(resp)
		if err != nil {
			return err
		}

		fmt.Println(string(prettyProto))
		return nil
	}

	fmt.Println(stringz.Join("\n\n", resp.SchemaText))
	return nil
}

func schemaWriteCmdFunc(cmd *cobra.Command, args []string) error {
	if len(args) == 0 && term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("must provide file path or contents via stdin")
	}

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

	client, err := authzed.NewClient(token.Endpoint, dialOptsFromFlags(cmd, token.APIToken)...)
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
		schemaBytes, err = ioutil.ReadAll(os.Stdin)
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
		prettyProto, err := prettyProto(resp)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to convert schema to JSON")
		}

		fmt.Println(string(prettyProto))
		return nil
	}

	return nil
}

func prettyProto(m proto.Message) ([]byte, error) {
	encoded, err := protojson.Marshal(m)
	if err != nil {
		return nil, err
	}
	var obj interface{}
	err = json.Unmarshal(encoded, &obj)
	if err != nil {
		panic("protojson decode failed: " + err.Error())
	}

	f := colorjson.NewFormatter()
	f.Indent = 2
	pretty, err := f.Marshal(obj)
	if err != nil {
		panic("colorjson encode failed: " + err.Error())
	}

	return pretty, nil
}

func clientForContext(cmd *cobra.Command, contextName string, secretStore storage.SecretStore) (*authzed.Client, error) {
	token, err := storage.GetToken(contextName, secretStore)
	if err != nil {
		return nil, err
	}
	log.Trace().Interface("token", token).Send()

	return authzed.NewClient(token.Endpoint, dialOptsFromFlags(cmd, token.APIToken)...)
}

func schemaCopyCmdFunc(cmd *cobra.Command, args []string) error {
	_, secretStore := defaultStorage()
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
		prettyProto, err := prettyProto(resp)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to convert schema to JSON")
		}

		fmt.Println(string(prettyProto))
		return nil
	}

	return nil
}

// rewriteSchema rewrites the given existing schema to include the specified prefix on all definitions.
func rewriteSchema(existingSchemaText string, definitionPrefix string) (string, error) {
	nsDefs, err := compiler.Compile([]compiler.InputSchema{
		{Source: input.Source("schema"), SchemaString: existingSchemaText},
	}, &definitionPrefix)
	if err != nil {
		return "", err
	}

	objectDefs := make([]string, 0, len(nsDefs))
	for _, nsDef := range nsDefs {
		objectDef, _ := generator.GenerateSource(nsDef)
		objectDefs = append(objectDefs, objectDef)
	}

	return strings.Join(objectDefs, "\n\n"), nil
}

// readSchema calls read schema for the client and returns the schema found.
func readSchema(client *authzed.Client) (string, error) {
	request := &v1.ReadSchemaRequest{}
	log.Trace().Interface("request", request).Msg("requesting schema read")

	resp, err := client.ReadSchema(context.Background(), request)
	if err != nil {
		errStatus, ok := status.FromError(err)
		if !ok || errStatus.Code() != codes.NotFound {
			return "", err
		}

		log.Debug().Msg("no schema defined")
		return "", nil
	}

	return resp.SchemaText, nil
}

// determinePrefixForSchema determines the prefix to be applied to a schema that will be written.
//
// If specifiedPrefix is non-empty, it is returned immediately.
// If existingSchema is non-nil, it is parsed for the prefix.
// Otherwise, the client is used to retrieve the existing schema (if any), and the prefix is retrieved from there.
func determinePrefixForSchema(specifiedPrefix string, client *authzed.Client, existingSchema *string) (string, error) {
	if specifiedPrefix != "" {
		return specifiedPrefix, nil
	}

	var schemaText string
	if existingSchema != nil {
		schemaText = *existingSchema
	} else {
		readSchemaText, err := readSchema(client)
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
	found, err := compiler.Compile([]compiler.InputSchema{
		{Source: input.Source("schema"), SchemaString: schemaText},
	}, &empty)
	if err != nil {
		return "", err
	}

	foundPrefixes := make([]string, 0, len(found))
	for _, def := range found {
		if strings.Contains(def.Name, "/") {
			parts := strings.Split(def.Name, "/")
			foundPrefixes = append(foundPrefixes, parts[0])
		} else {
			foundPrefixes = append(foundPrefixes, "")
		}
	}

	prefixes := stringz.Dedup(foundPrefixes)
	if len(prefixes) == 0 {
		return "", fmt.Errorf("found no schema definition prefixes")
	}

	if len(prefixes) > 1 {
		return "", fmt.Errorf("found multiple schema definition prefixes: %v", prefixes)
	}

	prefix := prefixes[0]
	log.Debug().Str("prefix", prefix).Msg("found schema definition prefix")
	return prefix, nil
}
