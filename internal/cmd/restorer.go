package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/cenkalti/backoff/v4"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/schollz/progressbar/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/pkg/backupformat"
)

// Fallback for datastore implementations on SpiceDB < 1.29.0 not returning proper gRPC codes
// Remove once https://github.com/authzed/spicedb/pull/1688 lands
var (
	txConflictCodes = []string{
		"SQLSTATE 23505",     // CockroachDB
		"Error 1062 (23000)", // MySQL
	}
	retryableErrorCodes = []string{
		"retryable error",                          // CockroachDB, PostgreSQL
		"try restarting transaction", "Error 1205", // MySQL
	}
)

type restorer struct {
	decoder               *backupformat.Decoder
	client                client.Client
	prefixFilter          string
	batchSize             int
	batchesPerTransaction int64
	skipOnConflicts       bool
	touchOnConflicts      bool
	disableRetryErrors    bool
	bar                   *progressbar.ProgressBar

	// stats
	filteredOutRels  int64
	writtenRels      int64
	writtenBatches   int64
	skippedRels      int64
	skippedBatches   int64
	duplicateRels    int64
	duplicateBatches int64
	totalRetries     int64
}

func newRestorer(decoder *backupformat.Decoder, client client.Client, prefixFilter string, batchSize int,
	batchesPerTransaction int64, skipOnConflicts bool, touchOnConflicts bool, disableRetryErrors bool,
) *restorer {
	return &restorer{
		decoder:               decoder,
		client:                client,
		prefixFilter:          prefixFilter,
		batchSize:             batchSize,
		batchesPerTransaction: batchesPerTransaction,
		skipOnConflicts:       skipOnConflicts,
		touchOnConflicts:      touchOnConflicts,
		disableRetryErrors:    disableRetryErrors,
		bar:                   relProgressBar("restoring from backup"),
	}
}

func (r *restorer) restoreFromDecoder(ctx context.Context) error {
	relationshipWriteStart := time.Now()
	defer func() {
		if err := r.bar.Finish(); err != nil {
			log.Warn().Err(err).Msg("error finalizing progress bar")
		}
	}()

	relationshipWriter, err := r.client.BulkImportRelationships(ctx)
	if err != nil {
		return fmt.Errorf("error creating writer stream: %w", err)
	}

	batch := make([]*v1.Relationship, 0, r.batchSize)
	batchesToBeCommitted := make([][]*v1.Relationship, 0, r.batchesPerTransaction)
	for rel, err := r.decoder.Next(); rel != nil && err == nil; rel, err = r.decoder.Next() {
		if err := ctx.Err(); err != nil {
			r.bar.Describe("backup restore aborted")
			return fmt.Errorf("aborted restore: %w", err)
		}

		if !hasRelPrefix(rel, r.prefixFilter) {
			r.filteredOutRels++
			continue
		}

		batch = append(batch, rel)

		if len(batch)%r.batchSize == 0 {
			batchesToBeCommitted = append(batchesToBeCommitted, batch)
			err := relationshipWriter.Send(&v1.BulkImportRelationshipsRequest{
				Relationships: batch,
			})
			if err != nil {
				// It feels non-idiomatic to check for error and perform an operation, but in gRPC, when an element
				// sent over the stream fails, we need to call recvAndClose() to get the error.
				if err := r.commitStream(ctx, relationshipWriter, batchesToBeCommitted); err != nil {
					return fmt.Errorf("error committing batches: %w", err)
				}

				// after an error
				relationshipWriter, err = r.client.BulkImportRelationships(ctx)
				if err != nil {
					return fmt.Errorf("error creating new writer stream: %w", err)
				}

				batchesToBeCommitted = batchesToBeCommitted[:0]
				batch = batch[:0]
				continue
			}

			// The batch just sent is kept in batchesToBeCommitted, which is used for retries.
			// Therefore, we cannot reuse the batch. Batches may fail on send, or on commit (CloseAndRecv).
			batch = make([]*v1.Relationship, 0, r.batchSize)

			// if we've sent the maximum number of batches per transaction, proceed to commit
			if int64(len(batchesToBeCommitted))%r.batchesPerTransaction != 0 {
				continue
			}

			if err := r.commitStream(ctx, relationshipWriter, batchesToBeCommitted); err != nil {
				return fmt.Errorf("error committing batches: %w", err)
			}

			relationshipWriter, err = r.client.BulkImportRelationships(ctx)
			if err != nil {
				return fmt.Errorf("error creating new writer stream: %w", err)
			}

			batchesToBeCommitted = batchesToBeCommitted[:0]
		}
	}

	// Write the last batch
	if len(batch) > 0 {
		// Since we are going to close the stream anyway after the last batch, and given the actual error
		// is only returned on CloseAndRecv(), we have to ignore the error here in order to get the actual
		// underlying error that caused Send() to fail. It also gives us the opportunity to retry it
		// in case it failed.
		batchesToBeCommitted = append(batchesToBeCommitted, batch)
		_ = relationshipWriter.Send(&v1.BulkImportRelationshipsRequest{Relationships: batch})
	}

	if err := r.commitStream(ctx, relationshipWriter, batchesToBeCommitted); err != nil {
		return fmt.Errorf("error committing last set of batches: %w", err)
	}

	r.bar.Describe("completed import")
	if err := r.bar.Finish(); err != nil {
		log.Warn().Err(err).Msg("error finalizing progress bar")
	}

	totalTime := time.Since(relationshipWriteStart)
	log.Info().
		Int64("batches", r.writtenBatches).
		Int64("relationships_loaded", r.writtenRels).
		Int64("relationships_skipped", r.skippedRels).
		Int64("duplicate_relationships", r.duplicateRels).
		Int64("relationships_filtered_out", r.filteredOutRels).
		Int64("retried_errors", r.totalRetries).
		Uint64("perSecond", perSec(uint64(r.writtenRels+r.skippedRels), totalTime)).
		Stringer("duration", totalTime).
		Msg("finished restore")
	return nil
}

func (r *restorer) commitStream(ctx context.Context, bulkImportClient v1.ExperimentalService_BulkImportRelationshipsClient,
	batchesToBeCommitted [][]*v1.Relationship,
) error {
	var numLoaded, expectedLoaded, retries uint64
	for _, b := range batchesToBeCommitted {
		expectedLoaded += uint64(len(b))
	}

	resp, err := bulkImportClient.CloseAndRecv() // transaction commit happens here

	// Failure to commit transaction means the stream is closed, so it can't be reused any further
	// The retry will be done using WriteRelationships instead of BulkImportRelationships
	// This lets us retry with TOUCH semantics in case of failure due to duplicates
	retryable := isRetryableError(err)
	conflict := isAlreadyExistsError(err)
	unknown := !retryable && !conflict && err != nil

	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		r.bar.Describe("backup restore aborted")
		return ctx.Err()
	case unknown:
		r.bar.Describe("failed with unrecoverable error")
		return fmt.Errorf("error finalizing write of %d batches: %w", len(batchesToBeCommitted), err)
	case retryable && r.disableRetryErrors:
		return err
	case conflict && r.skipOnConflicts:
		r.skippedRels += int64(expectedLoaded)
		r.skippedBatches += int64(len(batchesToBeCommitted))
		r.duplicateBatches += int64(len(batchesToBeCommitted))
		r.duplicateRels += int64(expectedLoaded)
		r.bar.Describe("skipping conflicting batch")
	case conflict && r.touchOnConflicts:
		r.bar.Describe("retrying conflicting batch")
		r.duplicateRels += int64(expectedLoaded)
		r.duplicateBatches += int64(len(batchesToBeCommitted))
		r.totalRetries++
		numLoaded, retries, err = r.writeBatchesWithRetry(ctx, batchesToBeCommitted)
		if err != nil {
			return fmt.Errorf("failed to write retried batch: %w", err)
		}

		retries++ // account for the initial attempt
		r.writtenBatches += int64(len(batchesToBeCommitted))
		r.writtenRels += int64(numLoaded)
	case conflict && (!r.touchOnConflicts && !r.skipOnConflicts):
		r.bar.Describe("conflict detected, aborting restore")
		return fmt.Errorf("duplicate relationships found")
	case retryable:
		r.bar.Describe("retrying after error")
		r.totalRetries++
		numLoaded, retries, err = r.writeBatchesWithRetry(ctx, batchesToBeCommitted)
		if err != nil {
			return fmt.Errorf("failed to write retried batch: %w", err)
		}

		retries++ // account for the initial attempt
		r.writtenBatches += int64(len(batchesToBeCommitted))
		r.writtenRels += int64(numLoaded)
	default:
		r.bar.Describe("restoring from backup")
		r.writtenBatches += int64(len(batchesToBeCommitted))
	}

	// it was a successful transaction commit without duplicates
	if resp != nil {
		r.writtenRels += int64(resp.NumLoaded)
		if expectedLoaded != resp.NumLoaded {
			log.Warn().Uint64("loaded", resp.NumLoaded).Uint64("expected", expectedLoaded).Msg("unexpected number of relationships loaded")
		}
	}

	if err := r.bar.Set64(r.writtenRels + r.skippedRels); err != nil {
		return fmt.Errorf("error incrementing progress bar: %w", err)
	}

	if !isatty.IsTerminal(os.Stderr.Fd()) {
		log.Trace().
			Int64("batches_written", r.writtenBatches).
			Int64("relationships_written", r.writtenRels).
			Int64("duplicate_batches", r.duplicateBatches).
			Int64("duplicate_relationships", r.duplicateRels).
			Int64("skipped_batches", r.skippedBatches).
			Int64("skipped_relationships", r.skippedRels).
			Uint64("retries", retries).
			Msg("restore progress")
	}

	return nil
}

// writeBatchesWithRetry writes a set of batches using touch semantics and without transactional guarantees -
// each batch will be committed independently. If a batch fails, it will be retried up to 10 times with a backoff.
func (r *restorer) writeBatchesWithRetry(ctx context.Context, batches [][]*v1.Relationship) (uint64, uint64, error) {
	backoffInterval := backoff.NewExponentialBackOff()
	backoffInterval.InitialInterval = 10 * time.Millisecond
	backoffInterval.MaxInterval = 2 * time.Second
	backoffInterval.MaxElapsedTime = 0
	backoffInterval.Reset()

	var currentRetries, totalRetries, loadedRels uint64
	for _, batch := range batches {
		updates := lo.Map[*v1.Relationship, *v1.RelationshipUpdate](batch, func(item *v1.Relationship, index int) *v1.RelationshipUpdate {
			return &v1.RelationshipUpdate{
				Relationship: item,
				Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
			}
		})

		for {
			// throttle the writes so we don't overwhelm the server
			time.Sleep(backoffInterval.NextBackOff())
			_, err := r.client.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{Updates: updates})

			if isRetryableError(err) && currentRetries < 10 {
				currentRetries++
				r.totalRetries++
				totalRetries++
				continue
			}
			if err != nil {
				return 0, 0, fmt.Errorf("error on attempting to WriteRelationships a previously failed batch: %w", err)
			}

			currentRetries = 0
			backoffInterval.Reset()
			loadedRels += uint64(len(batch))
			break
		}
	}

	return loadedRels, totalRetries, nil
}

func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}

	if s, ok := status.FromError(err); ok {
		if s.Code() == codes.AlreadyExists {
			return true
		}
	}

	for _, code := range txConflictCodes {
		if strings.Contains(err.Error(), code) {
			return true
		}
	}

	return false
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	if s, ok := status.FromError(err); ok {
		if s.Code() == codes.Unavailable {
			return true
		}
	}

	for _, code := range retryableErrorCodes {
		if strings.Contains(err.Error(), code) {
			return true
		}
	}

	return false
}

func perSec(i uint64, d time.Duration) uint64 {
	secs := uint64(d.Seconds())
	if secs == 0 {
		return i
	}
	return i / secs
}
