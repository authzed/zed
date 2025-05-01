package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ccoveille/go-safecast"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/caveats/types"
	newcompiler "github.com/authzed/spicedb/pkg/composableschemadsl/compiler"
	newinput "github.com/authzed/spicedb/pkg/composableschemadsl/input"
	"github.com/authzed/spicedb/pkg/diff"
	"github.com/authzed/spicedb/pkg/schemadsl/compiler"
	"github.com/authzed/spicedb/pkg/schemadsl/generator"
	"github.com/authzed/spicedb/pkg/schemadsl/input"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/commands"
	"github.com/authzed/zed/internal/console"
)

func registerAdditionalSchemaCmds(schemaCmd *cobra.Command) {
	schemaCmd.AddCommand(schemaCopyCmd)
	schemaCopyCmd.Flags().Bool("json", false, "output as JSON")
	schemaCopyCmd.Flags().String("schema-definition-prefix", "", "prefix to add to the schema's definition(s) before writing")

	schemaCmd.AddCommand(schemaWriteCmd)
	schemaWriteCmd.Flags().Bool("json", false, "output as JSON")
	schemaWriteCmd.Flags().String("schema-definition-prefix", "", "prefix to add to the schema's definition(s) before writing")

	schemaCmd.AddCommand(schemaDiffCmd)

	schemaCmd.AddCommand(schemaCompileCmd)
	schemaCompileCmd.Flags().String("out", "", "output filepath; omitting writes to stdout")
}

var schemaWriteCmd = &cobra.Command{
	Use:               "write <file?>",
	Args:              commands.ValidationWrapper(cobra.MaximumNArgs(1)),
	Short:             "Write a schema file (.zed or stdin) to the current permissions system",
	ValidArgsFunction: commands.FileExtensionCompletions("zed"),
	RunE:              schemaWriteCmdFunc,
}

var schemaCopyCmd = &cobra.Command{
	Use:               "copy <src context> <dest context>",
	Short:             "Copy a schema from one context into another",
	Args:              commands.ValidationWrapper(cobra.ExactArgs(2)),
	ValidArgsFunction: ContextGet,
	RunE:              schemaCopyCmdFunc,
}

var schemaDiffCmd = &cobra.Command{
	Use:   "diff <before file> <after file>",
	Short: "Diff two schema files",
	Args:  commands.ValidationWrapper(cobra.ExactArgs(2)),
	RunE:  schemaDiffCmdFunc,
}

var schemaCompileCmd = &cobra.Command{
	Use:   "compile <file>",
	Args:  commands.ValidationWrapper(cobra.ExactArgs(1)),
	Short: "Compile a schema that uses extended syntax into one that can be written to SpiceDB",
	Example: `
	Write to stdout:
		zed preview schema compile root.zed
	Write to an output file:
		zed preview schema compile root.zed --out compiled.zed
	`,
	ValidArgsFunction: commands.FileExtensionCompletions("zed"),
	RunE:              schemaCompileCmdFunc,
}

func schemaDiffCmdFunc(_ *cobra.Command, args []string) error {
	beforeBytes, err := os.ReadFile(args[0])
	if err != nil {
		return fmt.Errorf("failed to read before schema file: %w", err)
	}

	afterBytes, err := os.ReadFile(args[1])
	if err != nil {
		return fmt.Errorf("failed to read after schema file: %w", err)
	}

	before, err := compiler.Compile(
		compiler.InputSchema{Source: input.Source(args[0]), SchemaString: string(beforeBytes)},
		compiler.AllowUnprefixedObjectType(),
	)
	if err != nil {
		return err
	}

	after, err := compiler.Compile(
		compiler.InputSchema{Source: input.Source(args[1]), SchemaString: string(afterBytes)},
		compiler.AllowUnprefixedObjectType(),
	)
	if err != nil {
		return err
	}

	dbefore := diff.NewDiffableSchemaFromCompiledSchema(before)
	dafter := diff.NewDiffableSchemaFromCompiledSchema(after)

	schemaDiff, err := diff.DiffSchemas(dbefore, dafter, types.Default.TypeSet)
	if err != nil {
		return err
	}

	for _, ns := range schemaDiff.AddedNamespaces {
		console.Printf("Added definition: %s\n", ns)
	}

	for _, ns := range schemaDiff.RemovedNamespaces {
		console.Printf("Removed definition: %s\n", ns)
	}

	for nsName, ns := range schemaDiff.ChangedNamespaces {
		console.Printf("Changed definition: %s\n", nsName)
		for _, delta := range ns.Deltas() {
			console.Printf("\t %s: %s\n", delta.Type, delta.RelationName)
		}
	}

	for _, caveat := range schemaDiff.AddedCaveats {
		console.Printf("Added caveat: %s\n", caveat)
	}

	for _, caveat := range schemaDiff.RemovedCaveats {
		console.Printf("Removed caveat: %s\n", caveat)
	}

	return nil
}

func schemaCopyCmdFunc(cmd *cobra.Command, args []string) error {
	_, secretStore := client.DefaultStorage()
	srcClient, err := client.NewClientForContext(cmd, args[0], secretStore)
	if err != nil {
		return err
	}

	destClient, err := client.NewClientForContext(cmd, args[1], secretStore)
	if err != nil {
		return err
	}

	readRequest := &v1.ReadSchemaRequest{}
	log.Trace().Interface("request", readRequest).Msg("requesting schema read")

	readResp, err := srcClient.ReadSchema(cmd.Context(), readRequest)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to read schema")
	}
	log.Trace().Interface("response", readResp).Msg("read schema")

	prefix, err := determinePrefixForSchema(cmd.Context(), cobrautil.MustGetString(cmd, "schema-definition-prefix"), nil, &readResp.SchemaText)
	if err != nil {
		return err
	}

	schemaText, err := rewriteSchema(readResp.SchemaText, prefix)
	if err != nil {
		return err
	}

	writeRequest := &v1.WriteSchemaRequest{Schema: schemaText}
	log.Trace().Interface("request", writeRequest).Msg("writing schema")

	resp, err := destClient.WriteSchema(cmd.Context(), writeRequest)
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
	intFd, err := safecast.ToInt(uint(os.Stdout.Fd()))
	if err != nil {
		return err
	}
	if len(args) == 0 && term.IsTerminal(intFd) {
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

	prefix, err := determinePrefixForSchema(cmd.Context(), cobrautil.MustGetString(cmd, "schema-definition-prefix"), client, nil)
	if err != nil {
		return err
	}

	schemaText, err := rewriteSchema(string(schemaBytes), prefix)
	if err != nil {
		return err
	}

	request := &v1.WriteSchemaRequest{Schema: schemaText}
	log.Trace().Interface("request", request).Msg("writing schema")

	resp, err := client.WriteSchema(cmd.Context(), request)
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

	compiled, err := compiler.Compile(
		compiler.InputSchema{Source: input.Source("schema"), SchemaString: existingSchemaText},
		compiler.ObjectTypePrefix(definitionPrefix),
		compiler.SkipValidation(),
	)
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
func determinePrefixForSchema(ctx context.Context, specifiedPrefix string, client client.Client, existingSchema *string) (string, error) {
	if specifiedPrefix != "" {
		return specifiedPrefix, nil
	}

	var schemaText string
	if existingSchema != nil {
		schemaText = *existingSchema
	} else {
		readSchemaText, err := commands.ReadSchema(ctx, client)
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
	found, err := compiler.Compile(
		compiler.InputSchema{Source: input.Source("schema"), SchemaString: schemaText},
		compiler.AllowUnprefixedObjectType(),
		compiler.SkipValidation(),
	)
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

// Compiles an input schema written in the new composable schema syntax
// and produces it as a fully-realized schema
func schemaCompileCmdFunc(cmd *cobra.Command, args []string) error {
	stdOutFd, err := safecast.ToInt(uint(os.Stdout.Fd()))
	if err != nil {
		return err
	}
	outputFilepath := cobrautil.MustGetString(cmd, "out")
	if outputFilepath == "" && !term.IsTerminal(stdOutFd) {
		return fmt.Errorf("must provide stdout or output file path")
	}

	inputFilepath := args[0]
	inputSourceFolder := filepath.Dir(inputFilepath)
	var schemaBytes []byte
	schemaBytes, err = os.ReadFile(inputFilepath)
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}
	log.Trace().Str("schema", string(schemaBytes)).Str("file", args[0]).Msg("read schema from file")

	if len(schemaBytes) == 0 {
		return errors.New("attempted to compile empty schema")
	}

	compiled, err := newcompiler.Compile(newcompiler.InputSchema{
		Source:       newinput.Source(inputFilepath),
		SchemaString: string(schemaBytes),
	}, newcompiler.AllowUnprefixedObjectType(),
		newcompiler.SourceFolder(inputSourceFolder))
	if err != nil {
		return err
	}

	// Attempt to cast one kind of OrderedDefinition to another
	oldDefinitions := make([]compiler.SchemaDefinition, 0, len(compiled.OrderedDefinitions))
	for _, definition := range compiled.OrderedDefinitions {
		oldDefinition, ok := definition.(compiler.SchemaDefinition)
		if !ok {
			return fmt.Errorf("could not convert definition to old schemadefinition: %v", oldDefinition)
		}
		oldDefinitions = append(oldDefinitions, oldDefinition)
	}

	// This is where we functionally assert that the two systems are compatible
	generated, _, err := generator.GenerateSchema(oldDefinitions)
	if err != nil {
		return fmt.Errorf("could not generate resulting schema: %w", err)
	}

	// Add a newline at the end for hygiene's sake
	terminated := generated + "\n"

	if outputFilepath == "" {
		// Print to stdout
		fmt.Print(terminated)
	} else {
		err = os.WriteFile(outputFilepath, []byte(terminated), 0o_600)
		if err != nil {
			return err
		}
	}

	return nil
}
