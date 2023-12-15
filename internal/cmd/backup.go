package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/schemadsl/compiler"
	"github.com/authzed/spicedb/pkg/schemadsl/generator"
	"github.com/authzed/spicedb/pkg/tuple"
	"github.com/authzed/spicedb/pkg/typesystem"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/commands"
	"github.com/authzed/zed/pkg/backupformat"
)

var (
	backupCmd = &cobra.Command{
		Use:   "backup <filename>",
		Short: "Create, restore, and inspect Permissions System backups",
		Args:  cobra.ExactArgs(1),
		// Create used to be on the root, so add it here for back-compat.
		RunE: backupCreateCmdFunc,
	}

	backupCreateCmd = &cobra.Command{
		Use:   "create <filename>",
		Short: "Backup a permission system to a file",
		Args:  cobra.ExactArgs(1),
		RunE:  backupCreateCmdFunc,
	}

	backupRestoreCmd = &cobra.Command{
		Use:   "restore <filename>",
		Short: "Restore a permission system from a file",
		Args:  commands.StdinOrExactArgs(1),
		RunE:  restoreCmdFunc,
	}

	backupParseSchemaCmd = &cobra.Command{
		Use:   "parse-schema <filename>",
		Short: "Extract the schema from a backup file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return backupParseSchemaCmdFunc(cmd, os.Stdout, args)
		},
	}

	backupParseRevisionCmd = &cobra.Command{
		Use:   "parse-revision <filename>",
		Short: "Extract the revision from a backup file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return backupParseRevisionCmdFunc(cmd, os.Stdout, args)
		},
	}

	backupParseRelsCmd = &cobra.Command{
		Use:   "parse-relationships <filename>",
		Short: "Extract the relationships from a backup file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return backupParseRelsCmdFunc(cmd, os.Stdout, args)
		},
	}
)

func registerBackupCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(backupCmd)

	backupCmd.AddCommand(backupCreateCmd)
	backupCreateCmd.Flags().String("prefix-filter", "", "include only schema and relationships with a given prefix")
	backupCreateCmd.Flags().Bool("rewrite-legacy", false, "potentially modify the schema to exclude legacy/broken syntax")

	backupCmd.AddCommand(backupRestoreCmd)
	backupRestoreCmd.Flags().Int("batch-size", 1_000, "restore relationship write batch size")
	backupRestoreCmd.Flags().Int64("batches-per-transaction", 10, "number of batches per transaction")
	backupRestoreCmd.Flags().String("prefix-filter", "", "include only schema and relationships with a given prefix")
	backupRestoreCmd.Flags().Bool("rewrite-legacy", false, "potentially modify the schema to exclude legacy/broken syntax")

	// Restore used to be on the root, so add it there too, but hidden.
	rootCmd.AddCommand(&cobra.Command{
		Use:    "restore <filename>",
		Short:  "Restore a permission system from a file",
		Args:   cobra.MaximumNArgs(1),
		RunE:   restoreCmdFunc,
		Hidden: true,
	})

	backupCmd.AddCommand(backupParseSchemaCmd)
	backupParseSchemaCmd.Flags().String("prefix-filter", "", "include only schema and relationships with a given prefix")
	backupParseSchemaCmd.Flags().Bool("rewrite-legacy", false, "potentially modify the schema to exclude legacy/broken syntax")

	backupCmd.AddCommand(backupParseRevisionCmd)
	backupCmd.AddCommand(backupParseRelsCmd)
	backupParseRelsCmd.Flags().String("prefix-filter", "", "Include only relationships with a given prefix")
}

func createBackupFile(filename string) (*os.File, error) {
	if filename == "-" {
		log.Trace().Str("filename", "- (stdout)").Send()
		return os.Stdout, nil
	}

	log.Trace().Str("filename", filename).Send()

	if _, err := os.Stat(filename); err == nil {
		return nil, fmt.Errorf("backup file already exists: %s", filename)
	}

	f, err := os.Create(filename)
	if err != nil {
		return nil, fmt.Errorf("unable to create backup file: %w", err)
	}

	return f, nil
}

var (
	missingAllowedTypes = regexp.MustCompile(`(\s*)(relation)(.+)(/\* missing allowed types \*/)(.*)`)
	shortRelations      = regexp.MustCompile(`(\s*)relation [a-z][a-z0-9_]:(.+)`)
)

func partialPrefixMatch(name, prefix string) bool {
	return strings.HasPrefix(name, prefix+"/")
}

func filterSchemaDefs(schema, prefix string) (filteredSchema string, err error) {
	if schema == "" || prefix == "" {
		return schema, nil
	}

	compiledSchema, err := compiler.Compile(compiler.InputSchema{Source: "schema", SchemaString: schema}, compiler.SkipValidation())
	if err != nil {
		return "", fmt.Errorf("error reading schema: %w", err)
	}

	var prefixedDefs []compiler.SchemaDefinition
	for _, def := range compiledSchema.ObjectDefinitions {
		if partialPrefixMatch(def.Name, prefix) {
			prefixedDefs = append(prefixedDefs, def)
		}
	}
	for _, def := range compiledSchema.CaveatDefinitions {
		if partialPrefixMatch(def.Name, prefix) {
			prefixedDefs = append(prefixedDefs, def)
		}
	}

	if len(prefixedDefs) == 0 {
		return "", errors.New("filtered all definitions from schema")
	}

	filteredSchema, _, err = generator.GenerateSchema(prefixedDefs)
	if err != nil {
		return "", fmt.Errorf("error generating filtered schema: %w", err)
	}

	// Validate that the type system for the generated schema is comprehensive.
	compiledFilteredSchema, err := compiler.Compile(compiler.InputSchema{Source: "generated-schema", SchemaString: filteredSchema})
	if err != nil {
		return "", fmt.Errorf("generated invalid schema: %w", err)
	}

	for _, def := range compiledFilteredSchema.ObjectDefinitions {
		ts, err := typesystem.NewNamespaceTypeSystem(def, typesystem.ResolverForSchema(*compiledFilteredSchema))
		if err != nil {
			return "", fmt.Errorf("generated invalid schema: %w", err)
		}
		if _, err := ts.Validate(context.Background()); err != nil {
			return "", fmt.Errorf("generated invalid schema: %w", err)
		}
	}

	return
}

func hasRelPrefix(rel *v1.Relationship, prefix string) bool {
	// Skip any relationships without the prefix on both sides.
	return strings.HasPrefix(rel.Resource.ObjectType, prefix) &&
		strings.HasPrefix(rel.Subject.Object.ObjectType, prefix)
}

func relProgressBar(description string) *progressbar.ProgressBar {
	bar := progressbar.NewOptions(-1,
		progressbar.OptionSetWidth(10),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionSetVisibility(false),
	)
	if isatty.IsTerminal(os.Stderr.Fd()) {
		bar = progressbar.NewOptions64(-1,
			progressbar.OptionSetDescription(description),
			progressbar.OptionSetWriter(os.Stderr),
			progressbar.OptionSetWidth(10),
			progressbar.OptionThrottle(65*time.Millisecond),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
			progressbar.OptionSetItsString("relationship"),
			progressbar.OptionOnCompletion(func() { _, _ = fmt.Fprint(os.Stderr, "\n") }),
			progressbar.OptionSpinnerType(14),
			progressbar.OptionFullWidth(),
			progressbar.OptionSetRenderBlankState(true),
		)
	}
	return bar
}

func backupCreateCmdFunc(cmd *cobra.Command, args []string) (err error) {
	f, err := createBackupFile(args[0])
	if err != nil {
		return err
	}
	defer func(e *error) { *e = errors.Join(*e, f.Close()) }(&err)
	defer func(e *error) { *e = errors.Join(*e, f.Sync()) }(&err)

	client, err := client.NewClient(cmd)
	if err != nil {
		return fmt.Errorf("unable to initialize client: %w", err)
	}

	ctx := cmd.Context()
	schemaResp, err := client.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	if err != nil {
		return fmt.Errorf("error reading schema: %w", err)
	} else if schemaResp.ReadAt == nil {
		return fmt.Errorf("`backup` is not supported on this version of SpiceDB")
	}
	schema := schemaResp.SchemaText

	// Remove any invalid relations generated from old, backwards-incompat
	// Serverless permission systems.
	if cobrautil.MustGetBool(cmd, "rewrite-legacy") {
		schema = string(missingAllowedTypes.ReplaceAll([]byte(schema), []byte("\n/* deleted missing allowed type error */")))
		schema = string(shortRelations.ReplaceAll([]byte(schema), []byte("\n/* deleted short relation name */")))
	}

	// Skip any definitions without the provided prefix
	prefixFilter := cobrautil.MustGetString(cmd, "prefix-filter")
	if prefixFilter != "" {
		schema, err = filterSchemaDefs(schema, prefixFilter)
		if err != nil {
			return err
		}
	}

	encoder, err := backupformat.NewEncoder(f, schema, schemaResp.ReadAt)
	if err != nil {
		return fmt.Errorf("error creating backup file encoder: %w", err)
	}
	defer func(e *error) { *e = errors.Join(*e, encoder.Close()) }(&err)

	relationshipStream, err := client.BulkExportRelationships(ctx, &v1.BulkExportRelationshipsRequest{
		Consistency: &v1.Consistency{
			Requirement: &v1.Consistency_AtExactSnapshot{
				AtExactSnapshot: schemaResp.ReadAt,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("error exporting relationships: %w", err)
	}

	relationshipReadStart := time.Now()

	bar := relProgressBar("processing backup")
	var relsEncoded, relsProcessed uint
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("aborted backup: %w", err)
		}

		relsResp, err := relationshipStream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return fmt.Errorf("error receiving relationships: %w", err)
			}
			break
		}

		for _, rel := range relsResp.Relationships {
			if hasRelPrefix(rel, prefixFilter) {
				if err := encoder.Append(rel); err != nil {
					return fmt.Errorf("error storing relationship: %w", err)
				}
				relsEncoded++

				if relsEncoded%100_000 == 0 && !isatty.IsTerminal(os.Stderr.Fd()) {
					log.Trace().
						Uint("encoded", relsEncoded).
						Uint("processed", relsProcessed).
						Msg("backup progress")
				}
			}
			relsProcessed++
			if err := bar.Add(1); err != nil {
				return fmt.Errorf("error incrementing progress bar: %w", err)
			}
		}
	}
	totalTime := time.Since(relationshipReadStart)

	if err := bar.Finish(); err != nil {
		return fmt.Errorf("error finalizing progress bar: %w", err)
	}

	log.Info().
		Uint("encoded", relsEncoded).
		Uint("processed", relsProcessed).
		Uint64("perSecond", perSec(uint64(relsProcessed), totalTime)).
		Stringer("duration", totalTime).
		Msg("finished backup")

	return nil
}

func openRestoreFile(filename string) (*os.File, int64, error) {
	if filename == "" {
		log.Trace().Str("filename", "(stdin)").Send()
		return os.Stdin, -1, nil
	}

	log.Trace().Str("filename", filename).Send()

	stats, err := os.Stat(filename)
	if err != nil {
		return nil, 0, fmt.Errorf("unable to stat restore file: %w", err)
	}

	f, err := os.Open(filename)
	if err != nil {
		return nil, 0, fmt.Errorf("unable to open restore file: %w", err)
	}

	return f, stats.Size(), nil
}

func restoreCmdFunc(cmd *cobra.Command, args []string) error {
	decoder, closer, err := decoderFromArgs(cmd, args)
	if err != nil {
		return err
	}

	defer func(e *error) { *e = errors.Join(*e, closer.Close()) }(&err)
	defer func(e *error) { *e = errors.Join(*e, decoder.Close()) }(&err)

	if loadedToken := decoder.ZedToken(); loadedToken != nil {
		log.Debug().Str("revision", loadedToken.Token).Msg("parsed revision")
	}

	schema := decoder.Schema()

	// Remove any invalid relations generated from old, backwards-incompat
	// Serverless permission systems.
	if cobrautil.MustGetBool(cmd, "rewrite-legacy") {
		schema = string(missingAllowedTypes.ReplaceAll([]byte(schema), []byte("\n/* deleted missing allowed type error */")))
		schema = string(shortRelations.ReplaceAll([]byte(schema), []byte("\n/* deleted short relation name */")))
	}

	// Skip any definitions without the provided prefix
	prefixFilter := cobrautil.MustGetString(cmd, "prefix-filter")
	if prefixFilter != "" {
		schema, err = filterSchemaDefs(schema, prefixFilter)
		if err != nil {
			return err
		}
	}
	log.Debug().Str("schema", schema).Bool("filtered", prefixFilter != "").Msg("parsed schema")

	client, err := client.NewClient(cmd)
	if err != nil {
		return fmt.Errorf("unable to initialize client: %w", err)
	}

	ctx := cmd.Context()
	if _, err := client.WriteSchema(ctx, &v1.WriteSchemaRequest{
		Schema: schema,
	}); err != nil {
		return fmt.Errorf("unable to write schema: %w", err)
	}

	relationshipWriteStart := time.Now()

	relationshipWriter, err := client.BulkImportRelationships(ctx)
	if err != nil {
		return fmt.Errorf("error creating writer stream: %w", err)
	}

	batchSize := cobrautil.MustGetInt(cmd, "batch-size")
	batchesPerTransaction := cobrautil.MustGetInt64(cmd, "batches-per-transaction")

	batch := make([]*v1.Relationship, 0, batchSize)
	var written, batchesWritten int64
	bar := relProgressBar("restoring from backup")
	for rel, err := decoder.Next(); rel != nil && err == nil; rel, err = decoder.Next() {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("aborted restore: %w", err)
		}

		if !hasRelPrefix(rel, prefixFilter) {
			continue
		}

		batch = append(batch, rel)

		if len(batch)%batchSize == 0 {
			if err := relationshipWriter.Send(&v1.BulkImportRelationshipsRequest{
				Relationships: batch,
			}); err != nil {
				_, closeErr := relationshipWriter.CloseAndRecv()
				return fmt.Errorf("error sending batch to server: %w", errors.Join(err, closeErr))
			}

			// Reset the relationships in the batch
			batch = batch[:0]
			batchesWritten++

			if batchesWritten%batchesPerTransaction == 0 {
				resp, err := relationshipWriter.CloseAndRecv()
				if err != nil {
					return fmt.Errorf("error finalizing write of %d batches: %w", batchesPerTransaction, err)
				}
				written += int64(resp.NumLoaded)
				if err := bar.Set64(written); err != nil {
					return fmt.Errorf("error incrementing progress bar: %w", err)
				}

				if !isatty.IsTerminal(os.Stderr.Fd()) {
					log.Trace().
						Int64("batches", batchesWritten).
						Int64("relationships", written).
						Msg("restore progress")
				}

				relationshipWriter, err = client.BulkImportRelationships(ctx)
				if err != nil {
					return fmt.Errorf("error creating new writer stream: %w", err)
				}
			}
		}
	}

	// Write the last batch
	if err := relationshipWriter.Send(&v1.BulkImportRelationshipsRequest{
		Relationships: batch,
	}); err != nil {
		return fmt.Errorf("error sending last batch to server: %w", err)
	}

	// Finish the stream
	resp, err := relationshipWriter.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("error finalizing last write: %w", err)
	}
	batchesWritten++
	written += int64(resp.NumLoaded)
	if err := bar.Set64(written); err != nil {
		return fmt.Errorf("error incrementing progress bar: %w", err)
	}
	totalTime := time.Since(relationshipWriteStart)

	if err := bar.Finish(); err != nil {
		return fmt.Errorf("error finalizing progress bar: %w", err)
	}

	log.Info().
		Int64("batches", batchesWritten).
		Int64("relationships", written).
		Uint64("perSecond", perSec(uint64(written), totalTime)).
		Stringer("duration", totalTime).
		Msg("finished restore")

	return nil
}

func perSec(i uint64, d time.Duration) uint64 {
	secs := uint64(d.Seconds())
	if secs == 0 {
		return i
	}
	return i / secs
}

func backupParseSchemaCmdFunc(cmd *cobra.Command, out io.Writer, args []string) error {
	decoder, closer, err := decoderFromArgs(cmd, args)
	if err != nil {
		return err
	}

	defer func(e *error) { *e = errors.Join(*e, closer.Close()) }(&err)
	defer func(e *error) { *e = errors.Join(*e, decoder.Close()) }(&err)

	schema := decoder.Schema()

	// Remove any invalid relations generated from old, backwards-incompat
	// Serverless permission systems.
	if cobrautil.MustGetBool(cmd, "rewrite-legacy") {
		schema = string(missingAllowedTypes.ReplaceAll([]byte(schema), []byte("\n/* deleted missing allowed type error */")))
		schema = string(shortRelations.ReplaceAll([]byte(schema), []byte("\n/* deleted short relation name */")))
	}

	// Skip any definitions without the provided prefix
	if prefixFilter := cobrautil.MustGetString(cmd, "prefix-filter"); prefixFilter != "" {
		schema, err = filterSchemaDefs(schema, prefixFilter)
		if err != nil {
			return err
		}
	}

	_, err = fmt.Fprintln(out, schema)
	return err
}

func backupParseRevisionCmdFunc(cmd *cobra.Command, out io.Writer, args []string) error {
	decoder, closer, err := decoderFromArgs(cmd, args)
	if err != nil {
		return err
	}

	defer func(e *error) { *e = errors.Join(*e, closer.Close()) }(&err)
	defer func(e *error) { *e = errors.Join(*e, decoder.Close()) }(&err)

	loadedToken := decoder.ZedToken()
	if loadedToken == nil {
		return fmt.Errorf("failed to parse decoded revision")
	}

	_, err = fmt.Fprintln(out, loadedToken.Token)
	return err
}

func backupParseRelsCmdFunc(cmd *cobra.Command, out io.Writer, args []string) error {
	prefix := cobrautil.MustGetString(cmd, "prefix-filter")
	decoder, closer, err := decoderFromArgs(cmd, args)
	if err != nil {
		return err
	}

	defer func(e *error) { *e = errors.Join(*e, closer.Close()) }(&err)
	defer func(e *error) { *e = errors.Join(*e, decoder.Close()) }(&err)

	for rel, err := decoder.Next(); rel != nil && err == nil; rel, err = decoder.Next() {
		if !hasRelPrefix(rel, prefix) {
			continue
		}

		relString, err := tuple.StringRelationship(rel)
		if err != nil {
			return err
		}

		if _, err = fmt.Fprintln(out, replaceRelString(relString)); err != nil {
			return err
		}
	}

	return nil
}

func decoderFromArgs(_ *cobra.Command, args []string) (*backupformat.Decoder, io.Closer, error) {
	filename := "" // Default to stdin.
	if len(args) > 0 {
		filename = args[0]
	}

	f, _, err := openRestoreFile(filename)
	if err != nil {
		return nil, nil, err
	}

	decoder, err := backupformat.NewDecoder(f)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating restore file decoder: %w", err)
	}

	return decoder, f, nil
}

func replaceRelString(rel string) string {
	rel = strings.Replace(rel, "@", " ", 1)
	return strings.Replace(rel, "#", " ", 1)
}
