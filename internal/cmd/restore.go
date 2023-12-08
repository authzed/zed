package cmd

import (
	"fmt"
	"io"
	"os"
	"time"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/jzelinskie/cobrautil"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/pkg/backupformat"
)

func registerRestoreCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(restoreCmd)
	restoreCmd.Flags().Int("batch-size", 1_000, "restore relationship write batch size")
	restoreCmd.Flags().Int("batches-per-transaction", 10, "number of batches per transaction")
	restoreCmd.Flags().Bool("print-zedtoken-only", false, "just print the zedtoken and stop")
	restoreCmd.Flags().String("prefix-filter", "", "include only schema and relationships with a given prefix")
}

var restoreCmd = &cobra.Command{
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

	printZTOnly := cobrautil.MustGetBool(cmd, "print-zedtoken-only")

	var hasProgressbar bool
	var restoreReader io.Reader = f
	if isatty.IsTerminal(os.Stderr.Fd()) && !printZTOnly {
		bar := progressbar.DefaultBytes(fSize, "restoring")
		restoreReader = io.TeeReader(f, bar)
		hasProgressbar = true
	}

	decoder, err := backupformat.NewDecoder(restoreReader)
	if err != nil {
		return fmt.Errorf("error creating restore file decoder: %w", err)
	}

	if loadedToken := decoder.ZedToken(); loadedToken != nil {
		log.Info().Str("token", loadedToken.Token).Msg("printing ZedToken to stdout")
		fmt.Println(loadedToken.Token)
	}

	if printZTOnly {
		return nil
	}

	client, err := client.NewClient(cmd)
	if err != nil {
		return fmt.Errorf("unable to initialize client: %w", err)
	}

	ctx := cmd.Context()

	schema := decoder.Schema()
	prefixFilter := cobrautil.MustGetString(cmd, "prefix-filter")
	if prefixFilter != "" {
		schema, err = filterSchemaDefs(schema, prefixFilter)
		if err != nil {
			return err
		}
	}

	log.Debug().Str("schema", schema).Msg("writing schema")

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
				return fmt.Errorf("error sending batch to server: %w", err)
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
