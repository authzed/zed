package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jzelinskie/cobrautil/v2"
	"github.com/rodaine/table"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"golang.org/x/exp/constraints"
	"golang.org/x/exp/maps"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	corev1 "github.com/authzed/spicedb/pkg/proto/core/v1"
	schemapkg "github.com/authzed/spicedb/pkg/schema"
	"github.com/authzed/spicedb/pkg/schemadsl/compiler"
	"github.com/authzed/spicedb/pkg/schemadsl/generator"
	"github.com/authzed/spicedb/pkg/tuple"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/commands"
	"github.com/authzed/zed/internal/console"
	"github.com/authzed/zed/pkg/backupformat"
)

const doNotReturnIfExists = false

// cobraRunEFunc is the signature of a cobra.Command.RunE function.
type cobraRunEFunc = func(cmd *cobra.Command, args []string) (err error)

// TODO: apply these to other cmds as well
var errorHandlers = []func(error) error{
	addPermissionDeniedErrInfo,
	addSizeErrInfo,
}

// withErrorHandling is a wrapper that centralizes error handling, instead of having to scatter it around the command logic.
func withErrorHandling(f cobraRunEFunc) cobraRunEFunc {
	return func(cmd *cobra.Command, args []string) (err error) {
		cmdErr := f(cmd, args)
		for _, handler := range errorHandlers {
			cmdErr = handler(cmdErr)
		}
		return cmdErr
	}
}

func registerBackupCmd(rootCmd *cobra.Command) {
	backupCmd := &cobra.Command{
		Use:   "backup <filename>",
		Short: "Create, restore, and inspect permissions system backups",
		Args:  commands.ValidationWrapper(cobra.MaximumNArgs(1)),
		// Create used to be on the root, so add it here for back-compat.
		RunE: withErrorHandling(backupCreateCmdFunc),
	}

	backupCreateCmd := &cobra.Command{
		Use:   "create <filename>",
		Short: "Backup a permission system to a file",
		Args:  commands.ValidationWrapper(cobra.MaximumNArgs(1)),
		RunE:  withErrorHandling(backupCreateCmdFunc),
	}

	backupRestoreCmd := &cobra.Command{
		Use:   "restore <filename>",
		Short: "Restore a permission system from a file",
		Args:  commands.ValidationWrapper(commands.StdinOrExactArgs(1)),
		RunE:  backupRestoreCmdFunc,
	}

	backupParseSchemaCmd := &cobra.Command{
		Use:   "parse-schema <filename>",
		Short: "Extract the schema from a backup file",
		Args:  commands.ValidationWrapper(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			return backupParseSchemaCmdFunc(cmd, os.Stdout, args)
		},
	}

	backupParseRevisionCmd := &cobra.Command{
		Use:   "parse-revision <filename>",
		Short: "Extract the revision from a backup file",
		Args:  commands.ValidationWrapper(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			return backupParseRevisionCmdFunc(cmd, os.Stdout, args)
		},
	}

	backupParseRelsCmd := &cobra.Command{
		Use:   "parse-relationships <filename>",
		Short: "Extract the relationships from a backup file",
		Args:  commands.ValidationWrapper(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			return backupParseRelsCmdFunc(cmd, os.Stdout, args)
		},
	}

	backupRedactCmd := &cobra.Command{
		Use:   "redact <filename>",
		Short: "Redact a backup file to remove sensitive information",
		Args:  commands.ValidationWrapper(cobra.ExactArgs(1)),
		RunE:  backupRedactCmdFunc,
	}

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
	backupRedactCmd.Flags().Bool("print-redacted-object-ids", false, "prints the redacted object IDs")

	// Restore used to be on the root, so add it there too, but hidden.
	restoreCmd := &cobra.Command{
		Use:    "restore <filename>",
		Short:  "Restore a permission system from a backup file",
		Args:   commands.ValidationWrapper(cobra.MaximumNArgs(1)),
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
	cmd.Flags().Uint("batch-size", 1_000, "restore relationship write batch size")
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
	cmd.Flags().Uint32("page-limit", 0, "defines the number of relationships to be read by requested page during backup")
}

func createBackupFile(filename string, returnIfExists bool) (*os.File, bool, error) {
	if filename == "-" {
		log.Trace().Str("filename", "- (stdout)").Send()
		return os.Stdout, false, nil
	}

	log.Trace().Str("filename", filename).Send()

	if _, err := os.Stat(filename); err == nil {
		if !returnIfExists {
			return nil, false, fmt.Errorf("backup file already exists: %s", filename)
		}

		f, err := os.OpenFile(filename, os.O_RDWR|os.O_APPEND, 0o644)
		if err != nil {
			return nil, false, fmt.Errorf("unable to open existing backup file: %w", err)
		}

		return f, true, nil
	}

	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, false, fmt.Errorf("unable to create backup file: %w", err)
	}

	return f, false, nil
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

	compiledSchema, err := compiler.Compile(
		compiler.InputSchema{Source: "schema", SchemaString: schema},
		compiler.AllowUnprefixedObjectType(),
		compiler.SkipValidation(),
	)
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
	compiledFilteredSchema, err := compiler.Compile(
		compiler.InputSchema{Source: "generated-schema", SchemaString: filteredSchema},
		compiler.AllowUnprefixedObjectType(),
	)
	if err != nil {
		return "", fmt.Errorf("generated invalid schema: %w", err)
	}

	for _, rawDef := range compiledFilteredSchema.ObjectDefinitions {
		ts := schemapkg.NewTypeSystem(schemapkg.ResolverForCompiledSchema(*compiledFilteredSchema))
		def, err := schemapkg.NewDefinition(ts, rawDef)
		if err != nil {
			return "", fmt.Errorf("generated invalid schema: %w", err)
		}
		if _, err := def.Validate(context.Background()); err != nil {
			return "", fmt.Errorf("generated invalid schema: %w", err)
		}
	}

	return filteredSchema, nil
}

// hasRelPrefix returns false if any resources within the relationship do not
// contain the prefix.
func hasRelPrefix(rel *v1.Relationship, prefix string) bool {
	return strings.HasPrefix(rel.Resource.ObjectType, prefix) &&
		strings.HasPrefix(rel.Subject.Object.ObjectType, prefix)
}

// revisionForServerless determines the latest revision to use for the backup
// because Serverless doesn't return a revision in the ReadSchema response.
func revisionForServerless(ctx context.Context, spiceClient client.Client, schema *compiler.CompiledSchema) (*v1.ZedToken, error) {
	stream, err := spiceClient.ReadRelationships(ctx, &v1.ReadRelationshipsRequest{
		RelationshipFilter: &v1.RelationshipFilter{ResourceType: schema.ObjectDefinitions[0].Name},
		OptionalLimit:      1,
	})
	if err != nil {
		return nil, err
	}

	msg, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	log.Trace().Str("revision", msg.ReadAt.Token).Msg("determined serverless revision")
	return msg.ReadAt, nil
}

// CloseAndJoin attempts to close the provided arguement and joins the error
// with any existing errors that may have occurred.
//
// This function is intended to be used with `defer` like this:
// `defer CloseAndJoin(&err, f)`
func CloseAndJoin(e *error, maybeCloser any) {
	if closer, ok := maybeCloser.(io.Closer); ok {
		*e = errors.Join(*e, closer.Close())
	}
}

func backupCreateCmdFunc(cmd *cobra.Command, args []string) (err error) {
	prefixFilter := cobrautil.MustGetString(cmd, "prefix-filter")
	pageLimit := cobrautil.MustGetUint32(cmd, "page-limit")

	backupFileName, err := computeBackupFileName(cmd, args)
	if err != nil {
		return err
	}

	spiceClient, err := client.NewClient(cmd)
	if err != nil {
		return fmt.Errorf("unable to initialize client: %w", err)
	}

	return takeBackup(cmd.Context(), spiceClient, nil, backupFileName, prefixFilter, pageLimit)
}

func takeBackup(ctx context.Context, spiceClient client.Client, encoder backupformat.Encoder, backupFileName, prefixFilter string, pageLimit uint32) error {
 	schemaResp, err := spiceClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
 	if err != nil {
 		return fmt.Errorf("error reading schema: %w", err)
 	}
 
	// Determine if the server supports modern APIs for backups and if not,
	// fallback to using ReadSchema and ReadRelationships.
	// This codepath can be removed when AuthZed Serverless is fully sunset.
	if bulkOpsUnsupported := schemaResp.ReadAt == nil; bulkOpsUnsupported {
 		compiledSchema, err := compiler.Compile(
 			compiler.InputSchema{Source: "schema", SchemaString: schemaResp.SchemaText},
 			compiler.AllowUnprefixedObjectType(),
 			compiler.SkipValidation(),
 		)
 		if err != nil {
 			return err
 		}
 
 		revision, err := revisionForServerless(ctx, spiceClient, compiledSchema)
 		if err != nil {
 			return err
 		}
 
		var cursor string
		if encoder == nil {
			var fencoder *backupformat.OcfFileEncoder
			fencoder, cursor, err = backupformat.NewOrExistingFileEncoder(backupFileName, schemaResp.SchemaText, revision)
			if err != nil {
				return err
			}
			encoder = backupformat.WithProgress(prefixFilter, fencoder)
 		}
		defer CloseAndJoin(&err, encoder)
 
 		log.Trace().Strs("definitions", lo.Map(compiledSchema.ObjectDefinitions, func(def *corev1.NamespaceDefinition, _ int) string {
 			return def.Name
 		})).Msg("parsed object definitions")
 
 		var cursorObj string
 		for _, def := range compiledSchema.ObjectDefinitions {
 			req := &v1.ReadRelationshipsRequest{
 				RelationshipFilter: &v1.RelationshipFilter{ResourceType: def.Name},
 				OptionalLimit:      pageLimit,
 			}
 			if cursor != "" && cursorObj == def.Name {
 				req.OptionalCursor = &v1.Cursor{Token: cursor}
 			} else {
 				req.Consistency = &v1.Consistency{
 					Requirement: &v1.Consistency_AtExactSnapshot{
 						AtExactSnapshot: revision,
 					},
 				}
 			}
 			log.Trace().Str("resource", def.Name).Str("cursor", cursor).Str("revision", revision.Token).Msg("iterated over definition")
 
 			stream, err := spiceClient.ReadRelationships(ctx, req)
 			if err != nil {
 				return err
 			}
 
 			for msg, err := stream.Recv(); !errors.Is(err, io.EOF); msg, err = stream.Recv() {
 				switch {
 				case isCanceled(err) || isCanceled(ctx.Err()):
 					return context.Canceled
 				case isRetryableError(err):
 					newReq := req.CloneVT()
 					newReq.OptionalCursor = &v1.Cursor{Token: cursor}
 					stream, err = spiceClient.ReadRelationships(ctx, newReq)
 					if err != nil {
 						return fmt.Errorf("failed to retry request")
 					}
 				case err != nil:
 					return err
 				case ctx.Err() != nil:
 					return fmt.Errorf("aborted backup: %w", err)
 				default:
					cursor = msg.AfterResultCursor.Token
					cursorObj = def.Name
					log.Trace().Str("cursor", cursor).Stringer("relationship", msg.Relationship).Msg("appending relationship")
					if err := encoder.Append(msg.Relationship, cursor); err != nil {
						return err
					}
				}
			}
		}
		encoder.MarkComplete()
	} else {
		var cursor string
		if encoder == nil {
			encoder, cursor, err = backupformat.NewOrExistingFileEncoder(backupFileName, schemaResp.SchemaText, schemaResp.ReadAt)
			if err != nil {
				return err
			}
		}
		encoder = backupformat.WithProgress(prefixFilter, encoder)
		defer CloseAndJoin(&err, encoder)

		req := &v1.ExportBulkRelationshipsRequest{OptionalLimit: pageLimit}
		if cursor != "" {
			req.OptionalCursor = &v1.Cursor{Token: cursor}
		} else {
			req.Consistency = &v1.Consistency{
				Requirement: &v1.Consistency_AtExactSnapshot{
					AtExactSnapshot: schemaResp.ReadAt,
				},
			}
		}

		stream, err := spiceClient.ExportBulkRelationships(ctx, req)
		if err != nil {
			return err
		}

		for msg, err := stream.Recv(); !errors.Is(err, io.EOF); msg, err = stream.Recv() {
			switch {
			case isCanceled(err) || isCanceled(ctx.Err()):
				return context.Canceled
			case isRetryableError(err):
				newReq := req.CloneVT()
				newReq.OptionalCursor = &v1.Cursor{Token: cursor}
				stream, err = spiceClient.ExportBulkRelationships(ctx, newReq)
				if err != nil {
					return fmt.Errorf("failed to retry request")
				}
			case err != nil:
				return err
			case ctx.Err() != nil:
				return fmt.Errorf("aborted backup: %w", err)
			default:
				cursor = msg.AfterResultCursor.Token
				for _, r := range msg.Relationships {
					if err := encoder.Append(r, cursor); err != nil {
						return err
					}
				}
			}
		}
		encoder.MarkComplete()
	}

	// NOTE: err is returned here because there's cleanup being done
	// in the `defer` blocks that will modify the `err` if the cleanup
	// fails
	return err
}

// computeBackupFileName computes the backup file name based.
// If no file name is provided, it derives a backup on the current context
func computeBackupFileName(cmd *cobra.Command, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}

	configStore, secretStore := client.DefaultStorage()
	token, err := client.GetCurrentTokenWithCLIOverride(cmd, configStore, secretStore)
	if err != nil {
		return "", fmt.Errorf("failed to determine current zed context: %w", err)
	}

	ex, err := os.Executable()
	if err != nil {
		return "", err
	}
	exPath := filepath.Dir(ex)

	backupFileName := filepath.Join(exPath, token.Name+".zedbackup")

	return backupFileName, nil
}

func isCanceled(err error) bool {
	if st, ok := status.FromError(err); ok && st.Code() == codes.Canceled {
		return true
	}

	return errors.Is(err, context.Canceled)
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

	batchSize := cobrautil.MustGetUint(cmd, "batch-size")
	batchesPerTransaction := cobrautil.MustGetUint(cmd, "batches-per-transaction")

	strategy, err := GetEnum(cmd, "conflict-strategy", conflictStrategyMapping)
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
		return errors.New("failed to parse decoded revision")
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
	writer, _, err := createBackupFile(filename, doNotReturnIfExists)
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
	bar := console.CreateProgressBar("redacting backup")
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

	if len(redactor.RedactionMap().ObjectIDs) > 0 && cobrautil.MustGetBool(cmd, "print-redacted-object-ids") {
		tbl = table.New("Object ID", "Redacted Object ID")
		for k, v := range redactor.RedactionMap().ObjectIDs {
			tbl.AddRow(k, v)
		}
		tbl.Print()
		fmt.Println()
	}

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

		relString, err := tuple.V1StringRelationship(rel)
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

var sizeErrorRegEx = regexp.MustCompile(`received message larger than max \((\d+) vs. (\d+)\)`)

func addSizeErrInfo(err error) error {
	if err == nil {
		return nil
	}

	code := status.Code(err)
	if code != codes.ResourceExhausted {
		return err
	}

	if !strings.Contains(err.Error(), "received message larger than max") {
		return err
	}

	matches := sizeErrorRegEx.FindStringSubmatch(err.Error())
	if len(matches) != 3 {
		return fmt.Errorf("%w: set flag --max-message-size=bytecounthere to increase the maximum allowable size", err)
	}

	necessaryByteCount, atoiErr := strconv.Atoi(matches[1])
	if atoiErr != nil {
		return fmt.Errorf("%w: set flag --max-message-size=bytecounthere to increase the maximum allowable size", err)
	}

	return fmt.Errorf("%w: set flag --max-message-size=%d to increase the maximum allowable size", err, 2*necessaryByteCount)
}

func addPermissionDeniedErrInfo(err error) error {
	if err == nil {
		return nil
	}

	code := status.Code(err)
	if code != codes.PermissionDenied {
		return err
	}
	return fmt.Errorf("%w: ensure that the token used for this call has all required permissions", err)
}
