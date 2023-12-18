package cmd

import (
	"context"
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
	relsWritten    int64
	batchesWritten int64
	relsSkipped    int64
	duplicateRels  int64
	totalRetries   int64
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
			log.Err(err).Msg("error finalizing progress bar")
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
			return fmt.Errorf("aborted restore: %w", err)
		}

		if !hasRelPrefix(rel, r.prefixFilter) {
			continue
		}

		batch = append(batch, rel)

		if len(batch)%r.batchSize == 0 {
			batchesToBeCommitted = append(batchesToBeCommitted, batch)
			err := relationshipWriter.Send(&v1.BulkImportRelationshipsRequest{
				Relationships: batch,
			})
			if err != nil {
				r.totalRetries++

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

			// Reset the relationships in the batch. Do not reuse in case failure happens on subsequent batch in the tx
			batch = make([]*v1.Relationship, 0, r.batchSize)
			r.batchesWritten++

			// if we've sent the maximum number of batches per transaction, proceed to commit
			if r.batchesWritten%r.batchesPerTransaction != 0 {
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
		if err := relationshipWriter.Send(&v1.BulkImportRelationshipsRequest{
			Relationships: batch,
		}); err != nil {
			return fmt.Errorf("error sending last batch to server: %w", err)
		}
	}

	if err := r.commitStream(ctx, relationshipWriter, batchesToBeCommitted); err != nil {
		return fmt.Errorf("error committing last set of batches: %w", err)
	}

	r.bar.Describe("complected import")
	if err := r.bar.Finish(); err != nil {
		log.Err(err).Msg("error finalizing progress bar")
	}

	totalTime := time.Since(relationshipWriteStart)
	log.Info().
		Int64("batches", r.batchesWritten).
		Int64("relationships_loaded", r.relsWritten).
		Int64("relationships_skipped", r.relsSkipped).
		Int64("duplicate_relationships", r.duplicateRels).
		Int64("retried_errors", r.totalRetries).
		Uint64("perSecond", perSec(uint64(r.relsWritten), totalTime)).
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
	case unknown:
		r.bar.Describe("failed with unrecoverable error")
		return fmt.Errorf("error finalizing write of %d batches: %w", len(batchesToBeCommitted), err)
	case retryable && r.disableRetryErrors:
		return err
	case conflict && r.skipOnConflicts:
		r.relsSkipped += int64(expectedLoaded)
		r.duplicateRels += int64(expectedLoaded)
		numLoaded = expectedLoaded
		r.bar.Describe("skipping conflicting batch")
	case conflict && r.touchOnConflicts:
		r.bar.Describe("retrying conflicting batch")
		r.duplicateRels += int64(expectedLoaded)
		numLoaded, retries, err = r.writeBatchesWithRetry(ctx, batchesToBeCommitted)
		if err != nil {
			return fmt.Errorf("failed to write retried batch: %w", err)
		}
	case conflict && !r.touchOnConflicts:
		r.bar.Describe("conflict detected, aborting restore")
		return fmt.Errorf("duplicate relationships found")
	case retryable:
		r.bar.Describe("retrying after error")
		numLoaded, retries, err = r.writeBatchesWithRetry(ctx, batchesToBeCommitted)
		if err != nil {
			return fmt.Errorf("failed to write retried batch: %w", err)
		}
	default:
		r.bar.Describe("restoring from backup")
	}

	// it was a successful transaction commit without duplicates
	if resp != nil {
		numLoaded = resp.NumLoaded

		var expected uint64
		for _, b := range batchesToBeCommitted {
			expected += uint64(len(b))
		}

		if expected != numLoaded {
			log.Warn().Uint64("loaded", numLoaded).Uint64("expected", expected).Msg("unexpected number of relationships loaded")
		}
	}

	r.relsWritten += int64(numLoaded)
	if err := r.bar.Set64(r.relsWritten); err != nil {
		return fmt.Errorf("error incrementing progress bar: %w", err)
	}

	if !isatty.IsTerminal(os.Stderr.Fd()) {
		log.Trace().
			Int64("batches_written", r.batchesWritten).
			Int64("relationships_written", r.relsWritten).
			Int64("duplicate_relationships", r.duplicateRels).
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
		if s.Code() == codes.Unavailable {
			return true
		}
	}

	// FIXME temporary hack until a proper error is exposed from the API, specific to CRDB
	return strings.Contains(err.Error(), "SQLSTATE 23505")
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	if strings.Contains(err.Error(), "RETRY_SERIALIZABLE") { // FIXME hack until SpiceDB exposes proper typed err
		return true
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
