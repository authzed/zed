package cmd

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	newcompiler "github.com/authzed/spicedb/pkg/composableschemadsl/compiler"
	newinput "github.com/authzed/spicedb/pkg/composableschemadsl/input"
	"github.com/authzed/spicedb/pkg/schemadsl/compiler"
	"github.com/authzed/spicedb/pkg/schemadsl/generator"
	"github.com/ccoveille/go-safecast"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/authzed/zed/internal/commands"
)

func registerPreviewCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(previewCmd)

	previewCmd.AddCommand(schemaCmd)

	schemaCmd.AddCommand(schemaCompileCmd)
}

var previewCmd = &cobra.Command{
	Use:   "preview <subcommand>",
	Short: "Experimental commands that have been made available for preview",
}

var schemaCmd = &cobra.Command{
	Use:   "schema <subcommand>",
	Short: "Manage schema for a permissions system",
}

var schemaCompileCmd = &cobra.Command{
	Use:   "compile <file>",
	Args:  cobra.ExactArgs(1),
	Short: "Compile a schema that uses extended syntax into one that can be written to SpiceDB",
	// TODO: add longer example
	// TODO: is this correct?
	ValidArgsFunction: commands.FileExtensionCompletions("zed"),
	RunE:              schemaCompileCmdFunc,
}

// Compiles an input schema written in the new composable schema syntax
// and produces it as a fully-realized schema
func schemaCompileCmdFunc(cmd *cobra.Command, args []string) error {
	// TODO: should we maintain the validate semantics where you can provide any URL?
	stdOutFd, err := safecast.ToInt(uint(os.Stdout.Fd()))
	if err != nil {
		return err
	}
	outputFilepath := cobrautil.MustGetString(cmd, "out")
	if outputFilepath == "" && !term.IsTerminal(stdOutFd) {
		return fmt.Errorf("must provide stdout or output file path")
	}

	relativeInputFilepath := args[0]
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	inputFilepath := path.Join(cwd, relativeInputFilepath)
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
