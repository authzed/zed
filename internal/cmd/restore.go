package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/jzelinskie/cobrautil"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/backupformat"
	"github.com/authzed/zed/internal/client"
)

func registerRestoreCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(restoreCmd)
	restoreCmd.Flags().Int("batch-size", 1_000, "restore relationship write batch size")
	restoreCmd.Flags().Int("batches-per-transaction", 10, "number of batches per transaction")
}

var restoreCmd = &cobra.Command{
	Use:   "restore <filename>",
	Short: "Restore a permission system from a file",
	Args:  cobra.ExactArgs(1),
	RunE:  restoreCmdFunc,
}

func restoreCmdFunc(cmd *cobra.Command, args []string) error {
	filename := args[0]

	log.Trace().Str("filename", filename).Send()

	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("unable to open restore file: %w", err)
	}

	decoder, err := backupformat.NewDecoder(f)
	if err != nil {
		return fmt.Errorf("error creating restore file decoder: %w", err)
	}

	client, err := client.NewClient(cmd)
	if err != nil {
		return fmt.Errorf("unable to initialize client: %w", err)
	}

	ctx := context.Background()

	if _, err := client.WriteSchema(ctx, &v1.WriteSchemaRequest{
		Schema: decoder.Schema(),
	}); err != nil {
		return fmt.Errorf("unable to write schema: %w", err)
	}

	log.Debug().Msg("schema written")

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
				log.Debug().Uint64("relationships", written).Msg("relationships written")

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
