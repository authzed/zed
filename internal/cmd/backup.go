package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/pkg/backupformat"
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

func backupCmdFunc(cmd *cobra.Command, args []string) error {
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
	}

	var hasProgressbar bool
	var relWriter io.Writer = f
	if isatty.IsTerminal(os.Stderr.Fd()) {
		bar := progressbar.DefaultBytes(-1, "backing up")
		relWriter = io.MultiWriter(bar, f)
		hasProgressbar = true
	}

	encoder, err := backupformat.NewEncoder(relWriter, schemaResp.SchemaText, schemaResp.ReadAt)
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
			if err := encoder.Append(rel); err != nil {
				return fmt.Errorf("error storing relationship: %w", err)
			}
			stored++

			if stored%100_000 == 0 && !hasProgressbar {
				log.Trace().Uint("relationships", stored).Msg("relationships stored")
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
