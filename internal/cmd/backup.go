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
	"github.com/mattn/go-isatty"
	"github.com/rodaine/table"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/exp/constraints"
	"golang.org/x/exp/maps"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	schemapkg "github.com/authzed/spicedb/pkg/schema"
	"github.com/authzed/spicedb/pkg/schemadsl/compiler"
	"github.com/authzed/spicedb/pkg/schemadsl/generator"
	"github.com/authzed/spicedb/pkg/tuple"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/commands"
	"github.com/authzed/zed/internal/console"
	"github.com/authzed/zed/pkg/backupformat"
)

const (
	returnIfExists      = true
	doNotReturnIfExists = false
)

// BackupConfig holds the configuration for creating a backup.
type BackupConfig struct {
	// PrefixFilter filters relationships to only include those with this prefix.
	PrefixFilter string
	// PageLimit defines the number of relationships to be read per page during backup.
	PageLimit uint32
	// RewriteLegacy indicates whether to rewrite legacy schema syntax.
	RewriteLegacy bool
}

// ProgressTracker tracks backup progress for resumability.
type ProgressTracker interface {
	// GetCursor returns the stored cursor, or nil if no progress exists.
	GetCursor() *v1.Cursor
	// WriteCursor writes the current cursor to storage.
	WriteCursor(cursor *v1.Cursor) error
	// MarkComplete marks the backup as complete (e.g., removes progress file).
	MarkComplete() error
	// Close closes any underlying resources.
	Close() error
}

// fileProgressTracker implements ProgressTracker using a file.
type fileProgressTracker struct {
	file   *os.File
	cursor *v1.Cursor
}

func newFileProgressTracker(backupFileName string, backupAlreadyExisted bool) (*fileProgressTracker, error) {
	progressFileName := toLockFileName(backupFileName)
	var cursor *v1.Cursor
	var fileMode int

	readCursor, readErr := os.ReadFile(progressFileName)
	if backupAlreadyExisted {
		// Backup exists - we need a valid progress file to resume
		// Check for errors first (except not-exist) to avoid masking permission/I/O errors
		if readErr != nil && !os.IsNotExist(readErr) {
			return nil, fmt.Errorf("failed to read progress file for existing backup: %w", readErr)
		}
		if os.IsNotExist(readErr) || len(readCursor) == 0 {
			return nil, fmt.Errorf("backup file %s already exists", backupFileName)
		}
		// Successfully read the cursor
		cursor = &v1.Cursor{
			Token: string(readCursor),
		}
		// if backup existed and there is a progress marker, the latter should not be truncated
		fileMode = os.O_WRONLY | os.O_CREATE
		log.Info().Str("filename", backupFileName).Msg("backup file already exists, will resume")
	} else {
		// if a backup did not exist, make sure to truncate the progress file
		fileMode = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}

	progressFile, err := os.OpenFile(progressFileName, fileMode, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open progress file: %w", err)
	}

	return &fileProgressTracker{
		file:   progressFile,
		cursor: cursor,
	}, nil
}

func (f *fileProgressTracker) GetCursor() *v1.Cursor {
	return f.cursor
}

func (f *fileProgressTracker) WriteCursor(cursor *v1.Cursor) error {
	if cursor == nil {
		return errors.New("cannot write nil cursor to progress file")
	}

	if err := f.file.Truncate(0); err != nil {
		return fmt.Errorf("unable to truncate backup progress file: %w", err)
	}

	if _, err := f.file.Seek(0, 0); err != nil {
		return fmt.Errorf("unable to seek backup progress file: %w", err)
	}

	if _, err := f.file.WriteString(cursor.Token); err != nil {
		return fmt.Errorf("unable to write result cursor to backup progress file: %w", err)
	}

	// Sync to ensure cursor is durably persisted before continuing
	if err := f.file.Sync(); err != nil {
		return fmt.Errorf("unable to sync backup progress file: %w", err)
	}

	// Update in-memory cursor to keep it consistent with persisted state
	f.cursor = cursor

	return nil
}

func (f *fileProgressTracker) MarkComplete() error {
	// Check if already closed/completed
	if f.file == nil {
		return nil
	}

	// Store the filename before closing, as we need to remove after close
	// to ensure Windows compatibility (can't remove open files on Windows)
	filename := f.file.Name()

	// Close the file first to release the handle
	if err := f.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync progress file before removal: %w", err)
	}
	if err := f.file.Close(); err != nil {
		return fmt.Errorf("failed to close progress file before removal: %w", err)
	}
	f.file = nil // Mark as closed so Close() becomes a no-op

	// Now remove the file - log warning but don't fail the backup for cleanup issues
	if err := os.Remove(filename); err != nil {
		log.Warn().
			Str("progress-file", filename).
			Err(err).
			Msg("failed to remove progress file, consider removing it manually")
		// Don't return error - the backup succeeded, this is just cleanup
	}

	return nil
}

func (f *fileProgressTracker) Close() error {
	// Check if file is already closed (e.g., by MarkComplete)
	if f.file == nil {
		return nil
	}
	syncErr := f.file.Sync()
	closeErr := f.file.Close()
	f.file = nil
	return errors.Join(syncErr, closeErr)
}

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

func hasRelPrefix(rel *v1.Relationship, prefix string) bool {
	// Skip any relationships without the prefix on both sides.
	return strings.HasPrefix(rel.Resource.ObjectType, prefix) &&
		strings.HasPrefix(rel.Subject.Object.ObjectType, prefix)
}

func backupCreateCmdFunc(cmd *cobra.Command, args []string) (err error) {
	config := BackupConfig{
		PrefixFilter:  cobrautil.MustGetString(cmd, "prefix-filter"),
		PageLimit:     cobrautil.MustGetUint32(cmd, "page-limit"),
		RewriteLegacy: cobrautil.MustGetBool(cmd, "rewrite-legacy"),
	}

	backupFileName, err := computeBackupFileName(cmd, args)
	if err != nil {
		return err
	}

	backupFile, backupExists, err := createBackupFile(backupFileName, returnIfExists)
	if err != nil {
		return err
	}

	defer func(e *error) {
		*e = errors.Join(*e, backupFile.Sync())
		*e = errors.Join(*e, backupFile.Close())
	}(&err)

	progressTracker, err := newFileProgressTracker(backupFileName, backupExists)
	if err != nil {
		return err
	}
	defer func(e *error) { *e = errors.Join(*e, progressTracker.Close()) }(&err)

	spiceClient, err := client.NewClient(cmd)
	if err != nil {
		return fmt.Errorf("unable to initialize client: %w", err)
	}

	var zedToken *v1.ZedToken
	var encoder *backupformat.Encoder
	if backupExists {
		encoder, err = backupformat.NewEncoderForExisting(backupFile)
		if err != nil {
			return fmt.Errorf("error creating backup file encoder: %w", err)
		}
	} else {
		encoder, zedToken, err = encoderForNewBackupWithConfig(cmd.Context(), spiceClient, backupFile, config)
		if err != nil {
			return err
		}
	}

	defer func(e *error) { *e = errors.Join(*e, encoder.Close()) }(&err)

	backupCompleted, err := backupCreateImpl(cmd.Context(), spiceClient, encoder, progressTracker, config, zedToken)
	if err != nil {
		return err
	}

	if backupCompleted {
		if markErr := progressTracker.MarkComplete(); markErr != nil {
			err = errors.Join(err, markErr)
		}
	}

	return err
}

// backupCreateImpl performs the core backup logic. It is designed to be testable
// by accepting dependencies as parameters rather than creating them internally.
//
// Parameters:
//   - ctx: Context for cancellation
//   - spiceClient: The SpiceDB client to use for exporting relationships
//   - encoder: The encoder to write relationships to
//   - progressTracker: Tracks progress for resumability
//   - config: Backup configuration options
//   - zedToken: The token to use for consistency (nil if resuming from cursor)
//
// Returns:
//   - backupCompleted: true if the backup completed successfully
//   - err: any error that occurred during backup
func backupCreateImpl(
	ctx context.Context,
	spiceClient client.Client,
	encoder *backupformat.Encoder,
	progressTracker ProgressTracker,
	config BackupConfig,
	zedToken *v1.ZedToken,
) (backupCompleted bool, err error) {
	cursor := progressTracker.GetCursor()

	if zedToken == nil && cursor == nil {
		return false, errors.New("malformed existing backup, consider recreating it")
	}

	req := &v1.ExportBulkRelationshipsRequest{
		OptionalLimit:  config.PageLimit,
		OptionalCursor: cursor,
	}

	// if a cursor is present, zedtoken is not needed (it is already in the cursor)
	if zedToken != nil {
		req.Consistency = &v1.Consistency{
			Requirement: &v1.Consistency_AtExactSnapshot{
				AtExactSnapshot: zedToken,
			},
		}
	}

	relationshipReadStart := time.Now()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	progressBar := console.CreateProgressBar("processing backup")
	var relsFilteredOut, relsProcessed uint64
	defer func() {
		_ = progressBar.Finish()

		evt := log.Info().
			Uint64("filtered", relsFilteredOut).
			Uint64("processed", relsProcessed).
			Uint64("throughput", perSec(relsProcessed, time.Since(relationshipReadStart))).
			Stringer("elapsed", time.Since(relationshipReadStart).Round(time.Second))
		switch {
		case isCanceled(err):
			evt.Msg("backup canceled - resume by restarting the backup command")
		case err != nil:
			evt.Msg("backup failed")
		default:
			evt.Msg("finished backup")
		}
	}()

	err = takeBackup(ctx, spiceClient, req, func(response *v1.ExportBulkRelationshipsResponse) error {
		for _, rel := range response.Relationships {
			if hasRelPrefix(rel, config.PrefixFilter) {
				if err := encoder.Append(rel); err != nil {
					return fmt.Errorf("error storing relationship: %w", err)
				}
			} else {
				relsFilteredOut++
			}

			relsProcessed++
			if err := progressBar.Add(1); err != nil {
				return fmt.Errorf("error incrementing progress bar: %w", err)
			}

			// progress fallback in case there is no TTY
			if !isatty.IsTerminal(os.Stderr.Fd()) {
				select {
				case <-ticker.C:
					log.Info().
						Uint64("filtered", relsFilteredOut).
						Uint64("processed", relsProcessed).
						Uint64("throughput", perSec(relsProcessed, time.Since(relationshipReadStart))).
						Stringer("elapsed", time.Since(relationshipReadStart).Round(time.Second)).
						Msg("backup progress")
				default:
				}
			}
		}

		if response.AfterResultCursor != nil {
			if err := progressTracker.WriteCursor(response.AfterResultCursor); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

func takeBackup(ctx context.Context, spiceClient client.Client, req *v1.ExportBulkRelationshipsRequest, processResponse func(*v1.ExportBulkRelationshipsResponse) error) error {
	relationshipStream, err := spiceClient.ExportBulkRelationships(ctx, req)
	if err != nil {
		return fmt.Errorf("error exporting relationships: %w", err)
	}
	var lastResponse *v1.ExportBulkRelationshipsResponse
	for {
		if err := ctx.Err(); err != nil {
			if isCanceled(err) {
				return context.Canceled
			}

			return fmt.Errorf("aborted backup: %w", err)
		}

		relsResp, err := relationshipStream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			if isCanceled(err) {
				return context.Canceled
			}

			if isRetryableError(err) {
				// If the error is retryable, we overwrite the existing stream with a new
				// stream based on a new request that starts at the cursor location of the
				// last received response.

				// Clone the request to ensure that we are keeping all other fields the same
				newReq := req.CloneVT()
				cursorToken := "undefined"
				if lastResponse != nil && lastResponse.AfterResultCursor != nil {
					newReq.OptionalCursor = lastResponse.AfterResultCursor
					cursorToken = lastResponse.AfterResultCursor.Token
				}

				relationshipStream, err = spiceClient.ExportBulkRelationships(ctx, newReq)
				log.Info().Err(err).Str("cursor-token", cursorToken).Msg("encountered retryable error, resuming after last known cursor")
				// Bounce to the top of the loop
				continue
			}

			return fmt.Errorf("error receiving relationships: %w", err)
		}

		// Get a reference to the last response in case we need to retry
		// starting at its cursor
		lastResponse = relsResp

		// Process the response using the provided function
		err = processResponse(relsResp)
		if err != nil {
			return err
		}
	}

	return nil
}

// encoderForNewBackupWithConfig creates a new encoder for a new zed backup file using the provided config.
// It returns the ZedToken at which the backup must be taken.
func encoderForNewBackupWithConfig(ctx context.Context, c client.Client, backupFile *os.File, config BackupConfig) (*backupformat.Encoder, *v1.ZedToken, error) {
	schemaResp, err := c.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	if err != nil {
		return nil, nil, fmt.Errorf("error reading schema: %w", err)
	}
	if schemaResp.ReadAt == nil {
		return nil, nil, errors.New("`backup` is not supported on this version of SpiceDB")
	}
	schema := schemaResp.SchemaText

	// Remove any invalid relations generated from old, backwards-incompat
	// Serverless permission systems.
	if config.RewriteLegacy {
		schema = rewriteLegacy(schema)
	}

	// Skip any definitions without the provided prefix
	if config.PrefixFilter != "" {
		schema, err = filterSchemaDefs(schema, config.PrefixFilter)
		if err != nil {
			return nil, nil, err
		}
	}

	zedToken := schemaResp.ReadAt

	encoder, err := backupformat.NewEncoder(backupFile, schema, zedToken)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating backup file encoder: %w", err)
	}

	return encoder, zedToken, nil
}

func toLockFileName(backupFileName string) string {
	return backupFileName + ".lock"
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
