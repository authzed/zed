package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/ccoveille/go-safecast/v2"
	"github.com/cenkalti/backoff/v4"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/schollz/progressbar/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/spiceerrors"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/console"
	"github.com/authzed/zed/pkg/backupformat"
)

type ConflictStrategy int

const (
	Fail ConflictStrategy = iota
	Skip
	Touch

	defaultBackoff    = 50 * time.Millisecond
	defaultMaxRetries = 10
)

var conflictStrategyMapping = map[string]ConflictStrategy{
	"fail":  Fail,
	"skip":  Skip,
	"touch": Touch,
}

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
	rewriter              Rewriter
	decoder               *backupformat.Decoder
	client                client.Client
	batchSize             uint
	batchesPerTransaction uint
	conflictStrategy      ConflictStrategy
	disableRetryErrors    bool
	bar                   *progressbar.ProgressBar

	// stats
	filteredOutRels  uint
	writtenRels      uint
	writtenBatches   uint
	skippedRels      uint
	skippedBatches   uint
	duplicateRels    uint
	duplicateBatches uint
	totalRetries     uint
	requestTimeout   time.Duration
}

func newRestorer(rw Rewriter, decoder *backupformat.Decoder, client client.Client, batchSize uint,
	batchesPerTransaction uint, conflictStrategy ConflictStrategy, disableRetryErrors bool,
	requestTimeout time.Duration,
) *restorer {
	return &restorer{
		decoder:               decoder,
		rewriter:              rw,
		client:                client,
		requestTimeout:        requestTimeout,
		batchSize:             batchSize,
		batchesPerTransaction: batchesPerTransaction,
		conflictStrategy:      conflictStrategy,
		disableRetryErrors:    disableRetryErrors,
		bar:                   console.CreateProgressBar("restoring from backup"),
	}
}

func (r *restorer) restoreFromDecoder(ctx context.Context) error {
	schema, err := r.rewriter.RewriteSchema(r.decoder.Schema())
	if err != nil {
		return err
	}

	relationshipWriteStart := time.Now()
	defer func() {
		if err := r.bar.Finish(); err != nil {
			log.Warn().Err(err).Msg("error finalizing progress bar")
		}
	}()

	r.bar.Describe("restoring schema from backup")
	if _, err := r.client.WriteSchema(ctx, &v1.WriteSchemaRequest{
		Schema: schema,
	}); err != nil {
		return fmt.Errorf("unable to write schema: %w", err)
	}

	relationshipWriter, err := r.client.ImportBulkRelationships(ctx)
	if err != nil {
		return fmt.Errorf("error creating writer stream: %w", err)
	}

	r.bar.Describe("restoring relationships from backup")
	batch := make([]*v1.Relationship, 0, r.batchSize)
	batchesToBeCommitted := make([][]*v1.Relationship, 0, r.batchesPerTransaction)
	for rel, err := r.decoder.Next(); rel != nil && err == nil; rel, err = r.decoder.Next() {
		if err := ctx.Err(); err != nil {
			r.bar.Describe("backup restore aborted")
			return fmt.Errorf("aborted restore: %w", err)
		}

		rewritten, err := r.rewriter.RewriteRelationship(rel)
		if err != nil {
			return fmt.Errorf("failed to rewrite relationship: %w", err)
		} else if rewritten == nil {
			r.filteredOutRels++
			continue
		}
		batch = append(batch, rewritten)

		if uint(len(batch))%r.batchSize == 0 {
			batchesToBeCommitted = append(batchesToBeCommitted, batch)
			err := relationshipWriter.Send(&v1.ImportBulkRelationshipsRequest{
				Relationships: batch,
			})
			if err != nil {
				// It feels non-idiomatic to check for error and perform an operation, but in gRPC, when an element
				// sent over the stream fails, we need to call recvAndClose() to get the error.
				if err := r.commitStream(ctx, relationshipWriter, batchesToBeCommitted); err != nil {
					return fmt.Errorf("error committing batches: %w", err)
				}

				// after an error
				relationshipWriter, err = r.client.ImportBulkRelationships(ctx)
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
			if uint(len(batchesToBeCommitted))%r.batchesPerTransaction != 0 {
				continue
			}

			if err := r.commitStream(ctx, relationshipWriter, batchesToBeCommitted); err != nil {
				return fmt.Errorf("error committing batches: %w", err)
			}

			relationshipWriter, err = r.client.ImportBulkRelationships(ctx)
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
		_ = relationshipWriter.Send(&v1.ImportBulkRelationshipsRequest{Relationships: batch})
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
		Uint("batches", r.writtenBatches).
		Uint("relationships_loaded", r.writtenRels).
		Uint("relationships_skipped", r.skippedRels).
		Uint("duplicate_relationships", r.duplicateRels).
		Uint("relationships_filtered_out", r.filteredOutRels).
		Uint("retried_errors", r.totalRetries).
		Uint64("perSecond", perSec(uint64(r.writtenRels), totalTime)).
		Stringer("duration", totalTime).
		Msg("finished restore")
	return nil
}

func (r *restorer) commitStream(ctx context.Context, bulkImportClient v1.PermissionsService_ImportBulkRelationshipsClient,
	batchesToBeCommitted [][]*v1.Relationship,
) error {
	var numLoaded, expectedLoaded, retries uint
	for _, b := range batchesToBeCommitted {
		expectedLoaded += uint(len(b))
	}

	resp, err := bulkImportClient.CloseAndRecv() // transaction commit happens here

	// Failure to commit transaction means the stream is closed, so it can't be reused any further
	// The retry will be done using WriteRelationships instead of BulkImportRelationships
	// This lets us retry with TOUCH semantics in case of failure due to duplicates
	retryable := isRetryableError(err)
	conflict := isAlreadyExistsError(err)
	canceled, cancelErr := isCanceledError(ctx.Err(), err)
	unknown := !retryable && !conflict && !canceled && err != nil

	numBatches := uint(len(batchesToBeCommitted))

	switch {
	case canceled:
		r.bar.Describe("backup restore aborted")
		return cancelErr
	case unknown:
		r.bar.Describe("failed with unrecoverable error")
		return fmt.Errorf("error finalizing write of %d batches: %w", len(batchesToBeCommitted), err)
	case retryable && r.disableRetryErrors:
		return err
	case conflict && r.conflictStrategy == Skip:
		r.skippedRels += expectedLoaded
		r.skippedBatches += numBatches
		r.duplicateBatches += numBatches
		r.duplicateRels += expectedLoaded
		r.bar.Describe("skipping conflicting batch")
	case conflict && r.conflictStrategy == Touch:
		r.bar.Describe("touching conflicting batch")
		r.duplicateRels += expectedLoaded
		r.duplicateBatches += numBatches
		r.totalRetries++
		numLoaded, retries, err = r.writeBatchesWithRetry(ctx, batchesToBeCommitted)
		if err != nil {
			return fmt.Errorf("failed to write retried batch: %w", err)
		}

		retries++ // account for the initial attempt
		r.writtenBatches += numBatches
		r.writtenRels += numLoaded
	case conflict && r.conflictStrategy == Fail:
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
		r.writtenBatches += numBatches
		r.writtenRels += numLoaded
	default:
		r.bar.Describe("restoring relationships from backup")
		r.writtenBatches += numBatches
	}

	// it was a successful transaction commit without duplicates
	if resp != nil {
		numLoaded, err := safecast.Convert[uint](resp.NumLoaded)
		if err != nil {
			return spiceerrors.MustBugf("could not cast numLoaded to uint")
		}
		r.writtenRels += numLoaded
		if uint64(expectedLoaded) != resp.NumLoaded {
			log.Warn().Uint64("loaded", resp.NumLoaded).Uint("expected", expectedLoaded).Msg("unexpected number of relationships loaded")
		}
	}

	writtenAndSkipped, err := safecast.Convert[int64](r.writtenRels + r.skippedRels)
	if err != nil {
		return fmt.Errorf("too many written and skipped rels for an int64")
	}

	if err := r.bar.Set64(writtenAndSkipped); err != nil {
		return fmt.Errorf("error incrementing progress bar: %w", err)
	}

	if !isatty.IsTerminal(os.Stderr.Fd()) {
		log.Trace().
			Uint("batches_written", r.writtenBatches).
			Uint("relationships_written", r.writtenRels).
			Uint("duplicate_batches", r.duplicateBatches).
			Uint("duplicate_relationships", r.duplicateRels).
			Uint("skipped_batches", r.skippedBatches).
			Uint("skipped_relationships", r.skippedRels).
			Uint("retries", retries).
			Msg("restore progress")
	}

	return nil
}

// writeBatchesWithRetry writes a set of batches using touch semantics and without transactional guarantees -
// each batch will be committed independently. If a batch fails, it will be retried up to 10 times with a backoff.
func (r *restorer) writeBatchesWithRetry(ctx context.Context, batches [][]*v1.Relationship) (uint, uint, error) {
	backoffInterval := backoff.NewExponentialBackOff()
	backoffInterval.InitialInterval = defaultBackoff
	backoffInterval.MaxInterval = 2 * time.Second
	backoffInterval.MaxElapsedTime = 0
	backoffInterval.Reset()

	var currentRetries, totalRetries, loadedRels uint
	for _, batch := range batches {
		updates := lo.Map[*v1.Relationship, *v1.RelationshipUpdate](batch, func(item *v1.Relationship, _ int) *v1.RelationshipUpdate {
			return &v1.RelationshipUpdate{
				Relationship: item,
				Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
			}
		})

		for {
			cancelCtx, cancel := context.WithTimeout(ctx, r.requestTimeout)
			_, err := r.client.WriteRelationships(cancelCtx, &v1.WriteRelationshipsRequest{Updates: updates})
			cancel()

			if isRetryableError(err) && currentRetries < defaultMaxRetries {
				// throttle the writes so we don't overwhelm the server
				bo := backoffInterval.NextBackOff()
				r.bar.Describe(fmt.Sprintf("retrying write with backoff %s after error (attempt %d/%d)", bo,
					currentRetries+1, defaultMaxRetries))
				time.Sleep(bo)
				currentRetries++
				r.totalRetries++
				totalRetries++
				continue
			}
			if err != nil {
				return 0, 0, err
			}

			currentRetries = 0
			backoffInterval.Reset()
			loadedRels += uint(len(batch))
			break
		}
	}

	return loadedRels, totalRetries, nil
}

func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}

	if isGRPCCode(err, codes.AlreadyExists) {
		return true
	}

	return isContainsErrorString(err, txConflictCodes...)
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	if isGRPCCode(err, codes.Unavailable, codes.DeadlineExceeded) {
		return true
	}

	if isContainsErrorString(err, retryableErrorCodes...) {
		return true
	}

	return errors.Is(err, context.DeadlineExceeded)
}

func isCanceledError(errs ...error) (bool, error) {
	for _, err := range errs {
		if err == nil {
			continue
		}

		if errors.Is(err, context.Canceled) {
			return true, err
		}

		if isGRPCCode(err, codes.Canceled) {
			return true, err
		}
	}

	return false, nil
}

func isContainsErrorString(err error, errStrings ...string) bool {
	if err == nil {
		return false
	}

	for _, errString := range errStrings {
		if strings.Contains(err.Error(), errString) {
			return true
		}
	}

	return false
}

func isGRPCCode(err error, codes ...codes.Code) bool {
	if err == nil {
		return false
	}

	if s, ok := status.FromError(err); ok {
		if slices.Contains(codes, s.Code()) {
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
