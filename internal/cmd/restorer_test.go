package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/ccoveille/go-safecast/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/tuple"

	"github.com/authzed/zed/internal/client"
)

var (
	errUnrecoverable      = status.Error(codes.Internal, "unrecoverable")
	errRetryable          = status.Error(codes.Unavailable, "serialization")
	errConflict           = status.Error(codes.AlreadyExists, "conflict")
	oneUnrecoverableError = []error{errUnrecoverable}
	oneRetryableError     = []error{errRetryable}
	oneConflictError      = []error{errConflict}
)

func TestRestorer(t *testing.T) {
	for _, tt := range []struct {
		name                  string
		prefixFilter          string
		batchSize             uint
		batchesPerTransaction uint
		conflictStrategy      ConflictStrategy
		disableRetryErrors    bool
		sendErrors            []error
		commitErrors          []error
		touchErrors           []error
		relationships         []string
	}{
		{"honors batch size = 1", "", 1, 1, Fail, false, nil, nil, nil, testRelationships},
		{"correctly handles remainder batch", "", 2, 1, Fail, false, nil, nil, nil, testRelationships},
		{"correctly handles batch size == total rels", "", 3, 1, Fail, false, nil, nil, nil, testRelationships},
		{"correctly handles batch size > total rels", "", 4, 1, Fail, false, nil, nil, nil, testRelationships},
		{"correctly handles empty set", "", 1, 1, Fail, false, nil, nil, nil, nil},
		{"skips conflicting writes when skipOnConflict is enabled", "", 1, 1, Skip, false, nil, oneConflictError, nil, testRelationships},
		{"applies touch when touchOnConflict is enabled", "", 1, 1, Touch, false, nil, oneConflictError, nil, testRelationships},
		{"skips on conflict when skipOnConflict is enabled", "", 2, 1, Skip, false, nil, oneConflictError, nil, testRelationships},
		{"failed batches are written individually when touchOnConflict is enabled", "", 1, 2, Touch, false, nil, oneConflictError, nil, testRelationships},
		{"fails on conflict if touchOnConflict=false && skipOnConflict=false", "", 1, 1, Fail, false, oneConflictError, nil, nil, testRelationships},
		{"fails on unexpected commit error", "", 1, 1, Fail, false, nil, oneUnrecoverableError, nil, testRelationships},
		{"retries commit retryable errors", "", 1, 1, Fail, false, nil, oneRetryableError, nil, testRelationships},
		{"retries on conflict when fallback WriteRelationships fails", "", 1, 1, Touch, false, nil, oneConflictError, oneRetryableError, testRelationships},
		{"returns error on retryable error if retries are disabled", "", 1, 1, Fail, true, nil, oneRetryableError, nil, testRelationships},
		{"fails fast if conflict-triggered touch fails with an unrecoverable error", "", 1, 1, Touch, false, nil, oneConflictError, oneUnrecoverableError, testRelationships},
		{"retries if error happens right after sending a batch over the stream", "", 1, 1, Touch, false, oneConflictError, oneConflictError, nil, testRelationships},
		{"filters relationships", "test", 1, 1, Fail, false, nil, nil, nil, append([]string{"foo/resource:1#reader@foo/user:1"}, testRelationships...)},
		{"handles gracefully all rels as filtered", "invalid", 1, 1, Fail, false, nil, nil, nil, testRelationships},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			backupFileName := createTestBackup(t, testSchema, tt.relationships)
			d, closer, err := decoderFromArgs(backupFileName)
			require.NoError(err)
			t.Cleanup(func() {
				require.NoError(closer.Close())
			})

			expectedFilteredRels := make([]string, 0, len(tt.relationships))
			for _, rel := range tt.relationships {
				if !hasRelPrefix(tuple.MustParseV1Rel(rel), tt.prefixFilter) {
					continue
				}

				expectedFilteredRels = append(expectedFilteredRels, rel)
			}

			expectedBatches := uint(len(expectedFilteredRels)) / tt.batchSize
			// there is always one extra commit, regardless there is or not a remainder batch
			expectedCommits := expectedBatches/tt.batchesPerTransaction + 1
			remainderBatch := false
			if uint(len(expectedFilteredRels))%tt.batchSize != 0 {
				expectedBatches++
				remainderBatch = true
			}

			c := &mockClientForRestore{
				t:                              t,
				schema:                         testSchema,
				remainderBatch:                 remainderBatch,
				expectedRels:                   expectedFilteredRels,
				expectedBatches:                expectedBatches,
				requestedBatchSize:             tt.batchSize,
				requestedBatchesPerTransaction: tt.batchesPerTransaction,
				commitErrors:                   tt.commitErrors,
				touchErrors:                    tt.touchErrors,
				sendErrors:                     tt.sendErrors,
			}

			expectedConflicts := uint(0)
			expectedRetries := uint(0)
			var expectsError error
			for _, err := range tt.commitErrors {
				switch {
				case isRetryableError(err):
					{
						expectedRetries++
						if tt.disableRetryErrors {
							expectsError = err
						}
					}
				case isAlreadyExistsError(err):
					expectedConflicts++
				default:
					expectsError = err
				}
			}
			for _, err := range tt.touchErrors {
				if isRetryableError(err) {
					expectedRetries++
					if tt.disableRetryErrors {
						expectsError = err
					}
				} else {
					expectsError = err
				}
			}

			// if skip is enabled, there will be N less relationships written, where N is the number of conflicts
			expectedWrittenRels := uint(len(expectedFilteredRels))
			if tt.conflictStrategy == Skip {
				expectedWrittenRels -= expectedConflicts * tt.batchSize
			}

			expectedWrittenBatches := uint(len(expectedFilteredRels)) / tt.batchSize
			if tt.conflictStrategy == Skip {
				expectedWrittenBatches -= expectedConflicts
			}
			if remainderBatch {
				expectedWrittenBatches++
			}

			expectedTouchedBatches := expectedRetries
			expectedTouchedRels := expectedRetries * tt.batchSize
			if tt.conflictStrategy == Touch {
				expectedTouchedBatches += expectedConflicts * tt.batchesPerTransaction
				expectedTouchedRels += expectedConflicts * tt.batchesPerTransaction * tt.batchSize
			}

			expectedSkippedBatches := uint(0)
			expectedSkippedRels := uint(0)
			if tt.conflictStrategy == Skip {
				expectedSkippedBatches += expectedConflicts
				expectedSkippedRels += expectedConflicts * tt.batchSize
			}

			r := newRestorer(testSchema, d, c, tt.prefixFilter, tt.batchSize, tt.batchesPerTransaction, tt.conflictStrategy, tt.disableRetryErrors, 0*time.Second)
			err = r.restoreFromDecoder(t.Context())
			if expectsError != nil || (expectedConflicts > 0 && tt.conflictStrategy == Fail) {
				require.ErrorIs(err, expectsError)
				return
			}

			require.NoError(err)

			// assert on mock stats
			require.Equal(expectedBatches, c.receivedBatches, "unexpected number of received batches")
			require.Equal(expectedCommits, c.receivedCommits, "unexpected number of batch commits")
			require.Equal(uint(len(expectedFilteredRels)), c.receivedRels, "unexpected number of received relationships")
			require.Equal(expectedTouchedBatches, c.touchedBatches, "unexpected number of touched batches")
			require.Equal(expectedTouchedRels, c.touchedRels, "unexpected number of touched commits")

			// assert on restorer stats
			require.Equal(expectedWrittenRels, r.writtenRels, "unexpected number of written relationships")
			require.Equal(expectedWrittenBatches, r.writtenBatches, "unexpected number of written relationships")
			require.Equal(expectedSkippedBatches, r.skippedBatches, "unexpected number of conflicting batches skipped")
			require.Equal(expectedSkippedRels, r.skippedRels, "unexpected number of conflicting relationships skipped")
			require.Equal(expectedConflicts*tt.batchesPerTransaction, r.duplicateBatches, "unexpected number of duplicate batches detected")
			require.Equal(expectedConflicts*tt.batchesPerTransaction*tt.batchSize, r.duplicateRels, "unexpected number of duplicate relationships detected")
			require.Equal(expectedRetries+expectedConflicts-expectedSkippedBatches, r.totalRetries, "unexpected number of retries")
			require.Equal(uint(len(tt.relationships))-uint(len(expectedFilteredRels)), r.filteredOutRels, "unexpected number of filtered out relationships")
		})
	}
}

type mockClientForRestore struct {
	client.Client
	grpc.ClientStreamingClient[v1.ImportBulkRelationshipsRequest, v1.ImportBulkRelationshipsResponse]
	t                              *testing.T
	schema                         string
	remainderBatch                 bool
	expectedRels                   []string
	expectedBatches                uint
	requestedBatchSize             uint
	requestedBatchesPerTransaction uint
	receivedBatches                uint
	receivedCommits                uint
	receivedRels                   uint
	touchedBatches                 uint
	touchedRels                    uint
	lastReceivedBatch              []*v1.Relationship
	sendErrors                     []error
	commitErrors                   []error
	touchErrors                    []error
}

func (m *mockClientForRestore) Send(req *v1.ImportBulkRelationshipsRequest) error {
	m.receivedBatches++
	m.receivedRels += uint(len(req.Relationships))
	m.lastReceivedBatch = req.Relationships
	if m.receivedBatches <= uint(len(m.sendErrors)) {
		return m.sendErrors[m.receivedBatches-1]
	}

	if m.receivedBatches == m.expectedBatches && m.remainderBatch {
		require.Equal(m.t, uint(len(m.expectedRels))%m.requestedBatchSize, uint(len(req.Relationships)))
	} else {
		require.Equal(m.t, m.requestedBatchSize, uint(len(req.Relationships)))
	}

	for i, rel := range req.Relationships {
		// This is a gosec115 false positive which should be fixed in a future version.
		uinti, err := safecast.Convert[uint](i)
		if err != nil {
			// just in case to avoid accessing out of bounds in the []string
			uinti = 0
		}
		require.True(m.t, proto.Equal(rel, tuple.MustParseV1Rel(m.expectedRels[((m.receivedBatches-1)*m.requestedBatchSize)+uinti])))
	}

	return nil
}

func (m *mockClientForRestore) WriteRelationships(_ context.Context, in *v1.WriteRelationshipsRequest, _ ...grpc.CallOption) (*v1.WriteRelationshipsResponse, error) {
	m.touchedBatches++
	m.touchedRels += uint(len(in.Updates))
	if m.touchedBatches <= uint(len(m.touchErrors)) {
		return nil, m.touchErrors[m.touchedBatches-1]
	}

	return &v1.WriteRelationshipsResponse{}, nil
}

func (m *mockClientForRestore) CloseAndRecv() (*v1.ImportBulkRelationshipsResponse, error) {
	m.receivedCommits++
	lastBatch := m.lastReceivedBatch
	defer func() { m.lastReceivedBatch = nil }()

	if m.receivedCommits <= uint(len(m.commitErrors)) {
		return nil, m.commitErrors[m.receivedCommits-1]
	}

	return &v1.ImportBulkRelationshipsResponse{NumLoaded: uint64(len(lastBatch))}, nil
}

func (m *mockClientForRestore) ImportBulkRelationships(_ context.Context, _ ...grpc.CallOption) (grpc.ClientStreamingClient[v1.ImportBulkRelationshipsRequest, v1.ImportBulkRelationshipsResponse], error) {
	return m, nil
}

func (m *mockClientForRestore) WriteSchema(_ context.Context, wsr *v1.WriteSchemaRequest, _ ...grpc.CallOption) (*v1.WriteSchemaResponse, error) {
	require.Equal(m.t, m.schema, wsr.Schema, "unexpected schema in write schema request")
	return &v1.WriteSchemaResponse{}, nil
}
