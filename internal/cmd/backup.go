package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/backupformat"
	"github.com/authzed/zed/internal/client"
)

func registerBackupCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(backupCmd)
}

var backupCmd = &cobra.Command{
	Use:   "backup <filename>",
	Short: "Backup a permission system to a file",
	Args:  cobra.ExactArgs(1),
	RunE:  backupCmdFunc,
}

func backupCmdFunc(cmd *cobra.Command, args []string) error {
	filename := args[0]

	log.Trace().Str("filename", filename).Send()

	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("backup file already exists: %s", filename)
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("unable to create backup file: %w", err)
	}

	client, err := client.NewClient(cmd)
	if err != nil {
		return fmt.Errorf("unable to initialize client: %w", err)
	}

	ctx := context.Background()

	schemaResp, err := client.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	if err != nil {
		return fmt.Errorf("error reading schema: %w", err)
	}

	encoder, err := backupformat.NewEncoder(f, schemaResp.SchemaText)
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

	var stored uint
	for {
		relsResp, err := relationshipStream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return fmt.Errorf("error receiving relationships: %w", err)
			}
			break
		}

		for _, rel := range relsResp.Relationships {
			if err := encoder.Append(rel); err != nil {
				return fmt.Errorf("error storing relationship: %w", err)
			}
			stored++

			if stored%100_000 == 0 {
				log.Trace().Uint("relationships", stored).Msg("progress")
			}
		}
	}

	totalTime := time.Since(relationshipReadStart)
	relsPerSec := float64(stored) / totalTime.Seconds()

	log.Info().
		Uint("relationships", stored).
		Stringer("duration", totalTime).
		Float64("perSecond", relsPerSec).
		Msg("finished backup")

	if err := encoder.Close(); err != nil {
		return fmt.Errorf("error closing backup encoder: %w", err)
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("error syncing backup file: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("error closing backup file: %w", err)
	}

	return nil
}
