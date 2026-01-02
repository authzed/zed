package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ccoveille/go-safecast/v2"
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

type termChecker interface {
	IsTerminal(fd int) bool
}

type realTermChecker struct{}

func (rtc *realTermChecker) IsTerminal(fd int) bool {
	return term.IsTerminal(fd)
}

func registerAdditionalSchemaCmds(schemaCmd *cobra.Command) *cobra.Command {
	schemaWriteCmd := &cobra.Command{
		Use:               "write <file?>",
		Args:              commands.ValidationWrapper(cobra.MaximumNArgs(1)),
		Short:             "Write a schema file (.zed or stdin) to the current permissions system",
		ValidArgsFunction: commands.FileExtensionCompletions("zed"),
		Example: `
	Write from a file:
		zed schema write schema.zed
	Write from stdin:
		cat schema.zed | zed schema write
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := client.NewClient(cmd)
			if err != nil {
				return err
			}
			return schemaWriteCmdImpl(cmd, args, client, &realTermChecker{})
		},
	}

	schemaCopyCmd := &cobra.Command{
		Use:               "copy <src context> <dest context>",
		Short:             "Copy a schema from one context into another",
		Args:              commands.ValidationWrapper(cobra.ExactArgs(2)),
		ValidArgsFunction: ContextGet,
		RunE:              schemaCopyCmdFunc,
	}

	schemaDiffCmd := &cobra.Command{
		Use:   "diff <before file> <after file>",
		Short: "Diff two schema files",
		Args:  commands.ValidationWrapper(cobra.ExactArgs(2)),
		RunE:  schemaDiffCmdFunc,
	}

	schemaCompileCmd := &cobra.Command{
		Use:   "compile <file>",
		Args:  commands.ValidationWrapper(cobra.ExactArgs(1)),
		Short: "Compile a schema that uses extended syntax into one that can be written to SpiceDB",
		Example: `
	Write to stdout:
		zed preview schema compile root.zed
	Write to redirected stdout:
		zed preview schema compile schema.zed 1> compiled.zed
	Write to a file:
		zed preview schema compile root.zed --out compiled.zed
	`,
		ValidArgsFunction: commands.FileExtensionCompletions("zed"),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := schemaCompileOuter(cmd, args)
			return err
		},
	}

	schemaCmd.AddCommand(schemaCopyCmd)
	schemaCopyCmd.Flags().Bool("json", false, "output as JSON")
	schemaCopyCmd.Flags().String("schema-definition-prefix", "", "prefix to add to the schema's definition(s) before writing")

	schemaCmd.AddCommand(schemaWriteCmd)
	schemaWriteCmd.Flags().Bool("json", false, "output as JSON")
	schemaWriteCmd.Flags().String("schema-definition-prefix", "", "prefix to add to the schema's definition(s) before writing")

	schemaCmd.AddCommand(schemaDiffCmd)

	schemaCmd.AddCommand(schemaCompileCmd)
	schemaCompileCmd.Flags().String("out", "", "output filepath; omitting writes to stdout")

	return schemaCompileCmd
}

func schemaDiffCmdFunc(_ *cobra.Command, args []string) error {
	beforeReader, err := os.Open(args[0])
	if err != nil {
		return fmt.Errorf("failed to open before schema file: %w", err)
	}

	afterReader, err := os.Open(args[1])
	if err != nil {
		return fmt.Errorf("failed to open after schema file: %w", err)
	}

	return schemaDiffInner(
		beforeReader,
		afterReader,
		args[0],
		args[1],
		os.Stdout,
	)
}

func schemaDiffInner(beforeReader, afterReader io.Reader, beforeSource, afterSource string, writer io.Writer) error {
	beforeBytes, err := io.ReadAll(beforeReader)
	if err != nil {
		return fmt.Errorf("failed to read before schema: %w", err)
	}

	afterBytes, err := io.ReadAll(afterReader)
	if err != nil {
		return fmt.Errorf("failed to read after schema: %w", err)
	}

	before, err := compiler.Compile(
		compiler.InputSchema{Source: input.Source(beforeSource), SchemaString: string(beforeBytes)},
		compiler.AllowUnprefixedObjectType(),
	)
	if err != nil {
		return err
	}

	after, err := compiler.Compile(
		compiler.InputSchema{Source: input.Source(afterSource), SchemaString: string(afterBytes)},
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
		fmt.Fprintf(writer, "Added definition: %s\n", ns)
	}

	for _, ns := range schemaDiff.RemovedNamespaces {
		fmt.Fprintf(writer, "Removed definition: %s\n", ns)
	}

	for nsName, ns := range schemaDiff.ChangedNamespaces {
		fmt.Fprintf(writer, "Changed definition: %s\n", nsName)
		for _, delta := range ns.Deltas() {
			fmt.Fprintf(writer, "\t %s: %s\n", delta.Type, delta.RelationName)
		}
	}

	for _, caveat := range schemaDiff.AddedCaveats {
		fmt.Fprintf(writer, "Added caveat: %s\n", caveat)
	}

	for _, caveat := range schemaDiff.RemovedCaveats {
		fmt.Fprintf(writer, "Removed caveat: %s\n", caveat)
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

	prefix := cobrautil.MustGetString(cmd, "schema-definition-prefix")
	outputJSON := cobrautil.MustGetBool(cmd, "json")

	resp, err := schemaCopyInner(cmd.Context(), srcClient, destClient, prefix)
	if err != nil {
		return err
	}

	if outputJSON {
		prettyProto, err := commands.PrettyProto(resp)
		if err != nil {
			return fmt.Errorf("failed to convert schema to JSON: %w", err)
		}

		console.Println(string(prettyProto))
	}

	return nil
}

func schemaCopyInner(ctx context.Context, srcClient, destClient v1.SchemaServiceClient, definitionPrefix string) (*v1.WriteSchemaResponse, error) {
	readRequest := &v1.ReadSchemaRequest{}
	log.Trace().Interface("request", readRequest).Msg("requesting schema read")

	readResp, err := srcClient.ReadSchema(ctx, readRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema: %w", err)
	}
	log.Trace().Interface("response", readResp).Msg("read schema")

	prefix, err := determinePrefixForSchema(ctx, definitionPrefix, nil, &readResp.SchemaText)
	if err != nil {
		return nil, err
	}

	schemaText, err := rewriteSchema(readResp.SchemaText, prefix)
	if err != nil {
		return nil, err
	}

	writeRequest := &v1.WriteSchemaRequest{Schema: schemaText}
	log.Trace().Interface("request", writeRequest).Msg("writing schema")

	resp, err := destClient.WriteSchema(ctx, writeRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to write schema: %w", err)
	}
	log.Trace().Interface("response", resp).Msg("wrote schema")

	return resp, nil
}

func schemaWriteCmdImpl(cmd *cobra.Command, args []string, client v1.SchemaServiceClient, terminalChecker termChecker) error {
	stdInFd, err := safecast.Convert[int](os.Stdin.Fd())
	if err != nil {
		return err
	}

	if len(args) == 0 && terminalChecker.IsTerminal(stdInFd) {
		return errors.New("must provide file path or contents via stdin")
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
		return fmt.Errorf("failed to write schema: %w", err)
	}
	log.Trace().Interface("response", resp).Msg("wrote schema")

	if cobrautil.MustGetBool(cmd, "json") {
		prettyProto, err := commands.PrettyProto(resp)
		if err != nil {
			return fmt.Errorf("failed to convert schema to JSON: %w", err)
		}

		console.Println(string(prettyProto))
	}

	return nil
}

// rewriteSchema rewrites the given existing schema to include the specified prefix on all definitions and caveats.
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
func determinePrefixForSchema(ctx context.Context, specifiedPrefix string, client v1.SchemaServiceClient, existingSchema *string) (string, error) {
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

func schemaCompileOuter(cmd *cobra.Command, args []string) (bool, error) {
	outputFilepath := cobrautil.MustGetString(cmd, "out")

	var outputFile *os.File
	var toStdout bool
	switch outputFilepath {
	case "":
		toStdout = true
		outputFile = os.Stdout
	default:
		toStdout = false
		var err error
		outputFile, err = os.OpenFile(outputFilepath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return toStdout, fmt.Errorf("failed to create output file: %w", err)
		}
		defer func() {
			if err := outputFile.Close(); err != nil {
				log.Warn().Err(err).Msg("failed to close output file")
			}
		}()
	}

	return toStdout, schemaCompileInner(args, outputFile)
}

// Compiles an input schema written in the new composable schema syntax
// and produces it as a fully-realized schema
func schemaCompileInner(args []string, writer io.Writer) error {
	inputFilepath := args[0]
	inputSourceFolder := filepath.Dir(inputFilepath)
	schemaBytes, err := os.ReadFile(inputFilepath)
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

	_, err = fmt.Fprint(writer, terminated)
	if err != nil {
		return fmt.Errorf("failed to write schema: %w", err)
	}

	return nil
}
