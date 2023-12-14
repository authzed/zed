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
	"github.com/authzed/zed/pkg/backupformat"
)

var backupCmd = &cobra.Command{
	Use:   "backup <filename>",
	Short: "Create, restore, and inspect Permissions System backups",
	Args:  cobra.ExactArgs(1),
	// Create used to be on the root, so add it here for back-compat.
	RunE: backupCreateCmdFunc,
}

func registerBackupCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(backupCmd)

	backupCmd.AddCommand(backupCreateCmd)
	backupCreateCmd.Flags().String("prefix-filter", "", "include only schema and relationships with a given prefix")
	backupCreateCmd.Flags().Bool("rewrite-legacy", false, "potentially modify the schema to exclude legacy/broken syntax")

	backupCmd.AddCommand(backupRestoreCmd)
	backupRestoreCmd.Flags().Int("batch-size", 1_000, "restore relationship write batch size")
	backupRestoreCmd.Flags().Int("batches-per-transaction", 10, "number of batches per transaction")
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

var backupCreateCmd = &cobra.Command{
	Use:   "create <filename>",
	Short: "Backup a permission system to a file",
	Args:  cobra.ExactArgs(1),
	RunE:  backupCreateCmdFunc,
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
	missingAllowedTypes = regexp.MustCompile(`(\s*)(relation)(.+)(\/\* missing allowed types \*\/)(.*)`)
	shortRelations      = regexp.MustCompile(`(\s*)relation [a-z][a-z0-9_]:(.+)`)
)

func partialPrefixMatch(name, prefix string) bool {
	return strings.HasPrefix(name, prefix+"/")
}

func filterSchemaDefs(schema, prefix string) (filteredSchema string, err error) {
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

func backupCreateCmdFunc(cmd *cobra.Command, args []string) error {
	f, err := createBackupFile(args[0])
	if err != nil {
		return err
	}

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

	var hasProgressbar bool
	var relWriter io.Writer = f
	if isatty.IsTerminal(os.Stderr.Fd()) {
		bar := progressbar.DefaultBytes(-1, "backing up")
		relWriter = io.MultiWriter(bar, f)
		hasProgressbar = true
	}

	encoder, err := backupformat.NewEncoder(relWriter, schema, schemaResp.ReadAt)
	if err != nil {
		return fmt.Errorf("error creating backup file encoder: %w", err)
	}

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

	var processed uint
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
			if !hasRelPrefix(rel, prefixFilter) {
				continue
			}

			if err := encoder.Append(rel); err != nil {
				return fmt.Errorf("error storing relationship: %w", err)
			}
			processed++

			if processed%100_000 == 0 && !hasProgressbar {
				log.Trace().Uint("relationships", processed).Msg("relationships stored")
			}
		}
	}

	totalTime := time.Since(relationshipReadStart)
	relsPerSec := float64(processed) / totalTime.Seconds()

	log.Info().
		Uint("relationships", processed).
		Stringer("duration", totalTime).
		Float64("perSecond", relsPerSec).
		Msg("finished backup")

	if err := encoder.Close(); err != nil {
		return fmt.Errorf("error closing backup encoder: %w", err)
	}

	if f != os.Stdout {
		if err := f.Sync(); err != nil {
			return fmt.Errorf("error syncing backup file: %w", err)
		}
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("error closing backup file: %w", err)
	}

	return nil
}

var backupRestoreCmd = &cobra.Command{
	Use:   "restore <filename>",
	Short: "Restore a permission system from a file",
	Args:  cobra.MaximumNArgs(1),
	RunE:  restoreCmdFunc,
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
	filename := "" // Default to stdin.
	if len(args) > 0 {
		filename = args[0]
	}

	f, fSize, err := openRestoreFile(filename)
	if err != nil {
		return err
	}

	var hasProgressbar bool
	var restoreReader io.Reader = f
	if isatty.IsTerminal(os.Stderr.Fd()) {
		bar := progressbar.DefaultBytes(fSize, "restoring")
		restoreReader = io.TeeReader(f, bar)
		hasProgressbar = true
	}

	decoder, err := backupformat.NewDecoder(restoreReader)
	if err != nil {
		return fmt.Errorf("error creating restore file decoder: %w", err)
	}

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
	batchesPerTransaction := cobrautil.MustGetInt(cmd, "batches-per-transaction")

	batch := make([]*v1.Relationship, 0, batchSize)
	var written uint64
	var batchesWritten int
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
				if !hasProgressbar {
					log.Debug().Uint64("relationships", written).Msg("relationships written")
				}
				written += resp.NumLoaded

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

	written += resp.NumLoaded

	totalTime := time.Since(relationshipWriteStart)
	relsPerSec := float64(written) / totalTime.Seconds()

	log.Info().
		Uint64("relationships", written).
		Stringer("duration", totalTime).
		Float64("perSecond", relsPerSec).
		Msg("finished restore")

	if err := decoder.Close(); err != nil {
		return fmt.Errorf("error closing restore encoder: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("error closing restore file: %w", err)
	}

	return nil
}

var backupParseSchemaCmd = &cobra.Command{
	Use:   "parse-schema <filename>",
	Short: "Extract the schema from a backup file",
	Args:  cobra.ExactArgs(1),
	RunE:  backupParseSchemaCmdFunc,
}

func backupParseSchemaCmdFunc(cmd *cobra.Command, args []string) error {
	filename := "" // Default to stdin.
	if len(args) > 0 {
		filename = args[0]
	}

	f, _, err := openRestoreFile(filename)
	if err != nil {
		return err
	}

	decoder, err := backupformat.NewDecoder(f)
	if err != nil {
		return fmt.Errorf("error creating restore file decoder: %w", err)
	}
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

	fmt.Println(schema)
	return nil
}

var backupParseRevisionCmd = &cobra.Command{
	Use:   "parse-revision <filename>",
	Short: "Extract the revision from a backup file",
	Args:  cobra.ExactArgs(1),
	RunE:  backupParseRevisionCmdFunc,
}

func backupParseRevisionCmdFunc(_ *cobra.Command, args []string) error {
	filename := "" // Default to stdin.
	if len(args) > 0 {
		filename = args[0]
	}

	f, _, err := openRestoreFile(filename)
	if err != nil {
		return err
	}

	decoder, err := backupformat.NewDecoder(f)
	if err != nil {
		return fmt.Errorf("error creating restore file decoder: %w", err)
	}

	loadedToken := decoder.ZedToken()
	if loadedToken == nil {
		return fmt.Errorf("failed to parse decoded revision")
	}

	fmt.Println(loadedToken.Token)
	return nil
}

var backupParseRelsCmd = &cobra.Command{
	Use:   "parse-relationships <filename>",
	Short: "Extract the relationships from a backup file",
	Args:  cobra.ExactArgs(1),
	RunE:  backupParseRelsCmdFunc,
}

func backupParseRelsCmdFunc(cmd *cobra.Command, args []string) error {
	filename := "" // Default to stdin.
	if len(args) > 0 {
		filename = args[0]
	}

	f, _, err := openRestoreFile(filename)
	if err != nil {
		return err
	}

	decoder, err := backupformat.NewDecoder(f)
	if err != nil {
		return fmt.Errorf("error creating restore file decoder: %w", err)
	}

	for rel, err := decoder.Next(); rel != nil && err == nil; rel, err = decoder.Next() {
		if hasRelPrefix(rel, cobrautil.MustGetString(cmd, "prefix-filter")) {
			relString, err := tuple.StringRelationship(rel)
			if err != nil {
				return err
			}
			relString = strings.Replace(relString, "@", " ", 1)
			relString = strings.Replace(relString, "#", " ", 1)
			fmt.Println(relString)
		}
	}
	return nil
}
