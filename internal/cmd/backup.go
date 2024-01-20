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
	"github.com/rodaine/table"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"golang.org/x/exp/constraints"
	"golang.org/x/exp/maps"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/commands"
	"github.com/authzed/zed/pkg/backupformat"
)

var (
	backupCmd = &cobra.Command{
		Use:   "backup <filename>",
		Short: "Create, restore, and inspect permissions system backups",
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
		RunE:  backupRestoreCmdFunc,
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

	backupRedactCmd = &cobra.Command{
		Use:   "redact <filename>",
		Short: "Redact a backup file to remove sensitive information",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return backupRedactCmdFunc(cmd, args)
		},
	}
)

func registerBackupCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(backupCmd)
	registerBackupCreateFlags(backupCmd)

	backupCmd.AddCommand(backupCreateCmd)
	registerBackupCreateFlags(backupCreateCmd)

	backupCmd.AddCommand(backupRestoreCmd)
	registerBackupRestoreFlags(backupRestoreCmd)

	backupCmd.AddCommand(backupRedactCmd)
	backupRedactCmd.Flags().Bool("redact-definitions", true, "redact definitions")
	backupRedactCmd.Flags().Bool("redact-relations", true, "redact relations")
	backupRedactCmd.Flags().Bool("redact-object-ids", true, "redact object IDs")

	// Restore used to be on the root, so add it there too, but hidden.
	restoreCmd := &cobra.Command{
		Use:    "restore <filename>",
		Short:  "Restore a permission system from a backup file",
		Args:   cobra.MaximumNArgs(1),
		RunE:   backupRestoreCmdFunc,
		Hidden: true,
	}
	rootCmd.AddCommand(restoreCmd)
	registerBackupRestoreFlags(restoreCmd)

	backupCmd.AddCommand(backupParseSchemaCmd)
	backupParseSchemaCmd.Flags().String("prefix-filter", "", "include only schema and relationships with a given prefix")
	backupParseSchemaCmd.Flags().Bool("rewrite-legacy", false, "potentially modify the schema to exclude legacy/broken syntax")

	backupCmd.AddCommand(backupParseRevisionCmd)
	backupCmd.AddCommand(backupParseRelsCmd)
	backupParseRelsCmd.Flags().String("prefix-filter", "", "Include only relationships with a given prefix")
}

func registerBackupRestoreFlags(cmd *cobra.Command) {
	cmd.Flags().Int("batch-size", 1_000, "restore relationship write batch size")
	cmd.Flags().Uint("batches-per-transaction", 10, "number of batches per transaction")
	cmd.Flags().String("conflict-strategy", "fail", "strategy used when a conflicting relationship is found. Possible values: fail, skip, touch")
	cmd.Flags().Bool("disable-retries", false, "retries when an errors is determined to be retryable (e.g. serialization errors)")
	cmd.Flags().String("prefix-filter", "", "include only schema and relationships with a given prefix")
	cmd.Flags().Bool("rewrite-legacy", false, "potentially modify the schema to exclude legacy/broken syntax")
	cmd.Flags().Duration("request-timeout", 30*time.Second, "timeout for each request performed during restore")
}

func registerBackupCreateFlags(cmd *cobra.Command) {
	cmd.Flags().String("prefix-filter", "", "include only schema and relationships with a given prefix")
	cmd.Flags().Bool("rewrite-legacy", false, "potentially modify the schema to exclude legacy/broken syntax")
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
		schema = rewriteLegacy(schema)
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

func backupRestoreCmdFunc(cmd *cobra.Command, args []string) error {
	decoder, closer, err := decoderFromArgs(args...)
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
		schema = rewriteLegacy(schema)
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

	c, err := client.NewClient(cmd)
	if err != nil {
		return fmt.Errorf("unable to initialize client: %w", err)
	}

	batchSize := cobrautil.MustGetInt(cmd, "batch-size")
	batchesPerTransaction := cobrautil.MustGetUint(cmd, "batches-per-transaction")

	strategy, err := GetEnum[ConflictStrategy](cmd, "conflict-strategy", conflictStrategyMapping)
	if err != nil {
		return err
	}
	disableRetries := cobrautil.MustGetBool(cmd, "disable-retries")
	requestTimeout := cobrautil.MustGetDuration(cmd, "request-timeout")

	return newRestorer(schema, decoder, c, prefixFilter, batchSize, batchesPerTransaction, strategy,
		disableRetries, requestTimeout).restoreFromDecoder(cmd.Context())
}

// GetEnum is a helper for getting an enum value from a string cobra flag.
func GetEnum[E constraints.Integer](cmd *cobra.Command, name string, mapping map[string]E) (E, error) {
	value := cobrautil.MustGetString(cmd, name)
	value = strings.TrimSpace(strings.ToLower(value))
	if enum, ok := mapping[value]; ok {
		return enum, nil
	}

	var zeroValueE E
	return zeroValueE, fmt.Errorf("unexpected flag '%s' value '%s': should be one of %v", name, value, maps.Keys(mapping))
}

func backupParseSchemaCmdFunc(cmd *cobra.Command, out io.Writer, args []string) error {
	decoder, closer, err := decoderFromArgs(args...)
	if err != nil {
		return err
	}

	defer func(e *error) { *e = errors.Join(*e, closer.Close()) }(&err)
	defer func(e *error) { *e = errors.Join(*e, decoder.Close()) }(&err)

	schema := decoder.Schema()

	// Remove any invalid relations generated from old, backwards-incompat
	// Serverless permission systems.
	if cobrautil.MustGetBool(cmd, "rewrite-legacy") {
		schema = rewriteLegacy(schema)
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

func backupParseRevisionCmdFunc(_ *cobra.Command, out io.Writer, args []string) error {
	decoder, closer, err := decoderFromArgs(args...)
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

func backupRedactCmdFunc(cmd *cobra.Command, args []string) error {
	decoder, closer, err := decoderFromArgs(args...)
	if err != nil {
		return fmt.Errorf("error creating restore file decoder: %w", err)
	}

	defer func(e *error) { *e = errors.Join(*e, closer.Close()) }(&err)
	defer func(e *error) { *e = errors.Join(*e, decoder.Close()) }(&err)

	filename := args[0] + ".redacted"
	writer, err := createBackupFile(filename)
	if err != nil {
		return err
	}

	defer func(e *error) { *e = errors.Join(*e, writer.Close()) }(&err)

	redactor, err := backupformat.NewRedactor(decoder, writer, backupformat.RedactionOptions{
		RedactDefinitions: cobrautil.MustGetBool(cmd, "redact-definitions"),
		RedactRelations:   cobrautil.MustGetBool(cmd, "redact-relations"),
		RedactObjectIDs:   cobrautil.MustGetBool(cmd, "redact-object-ids"),
	})
	if err != nil {
		return fmt.Errorf("error creating redactor: %w", err)
	}

	defer func(e *error) { *e = errors.Join(*e, redactor.Close()) }(&err)
	bar := relProgressBar("redacting backup")
	var written int64
	for {
		if err := cmd.Context().Err(); err != nil {
			return fmt.Errorf("aborted redaction: %w", err)
		}

		err := redactor.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("error redacting: %w", err)
		}

		written++
		if err := bar.Set64(written); err != nil {
			return fmt.Errorf("error incrementing progress bar: %w", err)
		}
	}

	if err := bar.Finish(); err != nil {
		return fmt.Errorf("error finalizing progress bar: %w", err)
	}

	fmt.Println("Redaction map:")
	fmt.Println("--------------")
	fmt.Println()

	// Draw a table of definitions, caveats and relations mapped.
	tbl := table.New("Definition Name", "Redacted Name")
	for k, v := range redactor.RedactionMap().Definitions {
		tbl.AddRow(k, v)
	}

	tbl.Print()
	fmt.Println()

	if len(redactor.RedactionMap().Caveats) > 0 {
		tbl = table.New("Caveat Name", "Redacted Name")
		for k, v := range redactor.RedactionMap().Caveats {
			tbl.AddRow(k, v)
		}
		tbl.Print()
		fmt.Println()
	}

	tbl = table.New("Relation/Permission Name", "Redacted Name")
	for k, v := range redactor.RedactionMap().Relations {
		tbl.AddRow(k, v)
	}
	tbl.Print()
	fmt.Println()

	return nil
}

func backupParseRelsCmdFunc(cmd *cobra.Command, out io.Writer, args []string) error {
	prefix := cobrautil.MustGetString(cmd, "prefix-filter")
	decoder, closer, err := decoderFromArgs(args...)
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

func decoderFromArgs(args ...string) (*backupformat.Decoder, io.Closer, error) {
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

func rewriteLegacy(schema string) string {
	schema = string(missingAllowedTypes.ReplaceAll([]byte(schema), []byte("\n/* deleted missing allowed type error */")))
	return string(shortRelations.ReplaceAll([]byte(schema), []byte("\n/* deleted short relation name */")))
}
