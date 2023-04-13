//go:build wasm
// +build wasm

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"syscall/js"

	"github.com/authzed/spicedb/pkg/datastore"
	"github.com/authzed/spicedb/pkg/development"
	core "github.com/authzed/spicedb/pkg/proto/core/v1"
	devinterface "github.com/authzed/spicedb/pkg/proto/developer/v1"
	"github.com/authzed/spicedb/pkg/schemadsl/compiler"
	"github.com/authzed/spicedb/pkg/schemadsl/generator"
	"github.com/gookit/color"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/commands"
	"github.com/authzed/zed/internal/console"
)

// zedCommandResult is the struct JSON serialized to be returned to the caller.
type zedCommandResult struct {
	UpdatedContext string `json:"updated_context"`
	Output         string `json:"output"`
	Error          string `json:"error"`
}

func main() {
	rootCmd := buildRootCmd()

	// Force color output.
	color.ForceColor()

	// Set a local client and logger.
	client.NewClient = func(cmd *cobra.Command) (client.Client, error) {
		return wasmClient{}, nil
	}

	c := make(chan struct{}, 0)
	js.Global().Set("runZedCommand", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) != 2 {
			return fmt.Sprintf("invalid argument count %d; expected 2", len(args))
		}

		stringParams := make([]string, 0)
		params := args[1]
		length := params.Get("length").Int()

		for i := 0; i < length; i++ {
			stringParams = append(stringParams, params.Get(strconv.Itoa(i)).String())
		}

		result := runZedCommand(rootCmd, args[0].String(), stringParams)
		marshaled, err := json.Marshal(result)
		if err != nil {
			return `{"error": "could not marshal result"}`
		}

		return string(marshaled)
	}))
	fmt.Println("zed initialized")
	<-c
}

func buildRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "zed",
		Short: "SpiceDB client, by AuthZed",
		Long:  "A command-line client for managing SpiceDB clusters, built by AuthZed",
	}

	// Register shared commands.
	commands.RegisterPermissionCmd(rootCmd)
	commands.RegisterRelationshipCmd(rootCmd)
	commands.RegisterSchemaCmd(rootCmd)

	return rootCmd
}

// From: https://github.com/golang/debug/pull/8/files
func resetSubCommandFlagValues(root *cobra.Command) {
	for _, c := range root.Commands() {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Changed {
				f.Value.Set(f.DefValue)
				f.Changed = false
			}
		})
		resetSubCommandFlagValues(c)
	}
}

func runZedCommand(rootCmd *cobra.Command, requestContextJSON string, stringParams []string) zedCommandResult {
	ctx := context.Background()

	// Decode the request context.
	requestCtx := &devinterface.RequestContext{}
	err := protojson.Unmarshal([]byte(requestContextJSON), requestCtx)
	if err != nil {
		return zedCommandResult{Error: err.Error()}
	}

	// Build the new dev context.
	devCtx, devErrs, err := development.NewDevContext(ctx, requestCtx)
	if err != nil {
		return zedCommandResult{Error: err.Error()}
	}
	if devErrs != nil {
		return zedCommandResult{Error: "invalid schema or relationships"}
	}

	// Run the V1 API against the dev context.
	conn, stop, err := devCtx.RunV1InMemoryService()
	defer stop()
	if err != nil {
		return zedCommandResult{Error: err.Error()}
	}

	// Set a NewClient function which constructs a wasmClient which passes
	// all API calls to the in-memory V1 connection.
	client.NewClient = func(cmd *cobra.Command) (client.Client, error) {
		return wasmClient{
			conn: conn,
		}, nil
	}

	// Override Printf and Logger to collect the output.
	var buf bytes.Buffer
	console.Printf = func(format string, a ...any) {
		fmt.Fprintf(&buf, format, a...)
	}

	log.Logger = zerolog.New(&buf).With().Bool("is-log", true).Timestamp().Logger()

	// Set the input arguments.
	resetSubCommandFlagValues(rootCmd) // See: https://github.com/spf13/cobra/issues/1488
	rootCmd.SetArgs(stringParams)
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	// Execute the command.
	err = rootCmd.Execute()
	if err != nil {
		return zedCommandResult{Error: err.Error()}
	}

	// Collect the updated schema and relationships.
	headRev, err := devCtx.Datastore.HeadRevision(ctx)
	if err != nil {
		return zedCommandResult{Error: err.Error()}
	}

	reader := devCtx.Datastore.SnapshotReader(headRev)
	relationships := []*core.RelationTuple{}

	nsDefs, err := reader.ListAllNamespaces(ctx)
	if err != nil {
		return zedCommandResult{Error: err.Error()}
	}

	for _, nsDef := range nsDefs {
		it, err := reader.QueryRelationships(ctx, datastore.RelationshipsFilter{ResourceType: nsDef.Definition.Name})
		if err != nil {
			return zedCommandResult{Error: err.Error()}
		}
		defer it.Close()

		for rel := it.Next(); rel != nil; rel = it.Next() {
			relationships = append(relationships, rel)
		}
		it.Close()
	}

	caveatDefs, err := reader.ListAllCaveats(ctx)
	if err != nil {
		return zedCommandResult{Error: err.Error()}
	}

	schemaDefinitions := make([]compiler.SchemaDefinition, 0, len(nsDefs)+len(caveatDefs))
	for _, caveatDef := range caveatDefs {
		schemaDefinitions = append(schemaDefinitions, caveatDef.Definition)
	}

	for _, nsDef := range nsDefs {
		schemaDefinitions = append(schemaDefinitions, nsDef.Definition)
	}

	schemaText, _, err := generator.GenerateSchema(schemaDefinitions)
	if err != nil {
		return zedCommandResult{Error: err.Error()}
	}

	// Build the updated request context.
	updatedRequestCtx := &devinterface.RequestContext{
		Schema:        schemaText,
		Relationships: relationships,
	}

	encodedUpdatedContext, err := protojson.Marshal(updatedRequestCtx)
	if err != nil {
		return zedCommandResult{Error: err.Error()}
	}

	return zedCommandResult{UpdatedContext: string(encodedUpdatedContext), Output: buf.String()}
}
