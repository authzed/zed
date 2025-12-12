package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/genutil/mapz"
	"github.com/authzed/spicedb/pkg/tuple"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/storage"
	zedtesting "github.com/authzed/zed/internal/testing"
	"github.com/authzed/zed/pkg/backupformat"
)

func init() {
	zerolog.SetGlobalLevel(zerolog.Disabled)
}

const testSchema = `definition test/resource {
	relation reader: test/user
}

definition test/user {}`

var testRelationships = []string{
	`test/resource:1#reader@test/user:1`,
	`test/resource:2#reader@test/user:2`,
	`test/resource:3#reader@test/user:3`,
}

func TestFilterSchemaDefs(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name         string
		inputSchema  string
		inputPrefix  string
		outputSchema string
		err          string
	}{
		{
			name:         "empty schema returns as is",
			inputSchema:  "",
			inputPrefix:  "",
			outputSchema: "",
			err:          "",
		},
		{
			name:         "no input prefix matches everything",
			inputSchema:  "definition test {}",
			inputPrefix:  "",
			outputSchema: "definition test {}",
			err:          "",
		},
		{
			name:         "filter over schema without prefix filters everything",
			inputSchema:  "definition test {}",
			inputPrefix:  "myprefix",
			outputSchema: "",
			err:          "filtered all definitions from schema",
		},
		{
			name:         "filter over schema with same prefix returns schema as is",
			inputSchema:  "definition myprefix/test {}\n\ndefinition myprefix/test2 {}",
			inputPrefix:  "myprefix",
			outputSchema: "definition myprefix/test {}\n\ndefinition myprefix/test2 {}",
			err:          "",
		},
		{
			name:         "filter over schema with different prefixes filters non matching namespaces",
			inputSchema:  "definition myprefix/test {}\n\ndefinition myprefix2/test2 {}",
			inputPrefix:  "myprefix",
			outputSchema: "definition myprefix/test {}",
			err:          "",
		},
		{
			name:         "filter over schema caveats with same prefixes returns as is",
			inputSchema:  "caveat myprefix/one(a int) {\n\ta == 1\n}",
			inputPrefix:  "myprefix",
			outputSchema: "caveat myprefix/one(a int) {\n\ta == 1\n}",
			err:          "",
		},
		{
			name:         "filter over unprefixed schema caveats with a prefix filters out",
			inputSchema:  "caveat one(a int) {\n\ta == 1\n}",
			inputPrefix:  "myprefix",
			outputSchema: "",
			err:          "filtered all definitions from schema",
		},
		{
			name:         "filter over schema mixed prefixed/unprefixed caveats filters out",
			inputSchema:  "caveat one(a int) {\n\ta == 1\n}\n\ncaveat myprefix/two(a int) {\n\ta == 2\n}",
			inputPrefix:  "myprefix",
			outputSchema: "caveat myprefix/two(a int) {\n\ta == 2\n}",
			err:          "",
		},
		{
			name:         "filter over schema namespaces and caveats with same prefixes returns as is",
			inputSchema:  "definition myprefix/test {}\n\ncaveat myprefix/one(a int) {\n\ta == 1\n}",
			inputPrefix:  "myprefix",
			outputSchema: "definition myprefix/test {}\n\ncaveat myprefix/one(a int) {\n\ta == 1\n}",
			err:          "",
		},
		{
			name:         "fails on invalid schema",
			inputSchema:  "definition a/test {}\n\ncaveat a/one(a int) {\n\ta == 1\n}",
			inputPrefix:  "a",
			outputSchema: "definition a/test {}\n\ncaveat a/one(a int) {\n\ta == 1\n}",
			err:          "value does not match regex pattern",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt := tt
			t.Parallel()

			outputSchema, err := filterSchemaDefs(tt.inputSchema, tt.inputPrefix)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.outputSchema, outputSchema)
			}
		})
	}
}

func TestBackupParseRelsCmdFunc(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		filter        string
		schema        string
		relationships []string
		output        []string
		err           string
	}{
		{
			name:          "basic test",
			filter:        "test",
			schema:        testSchema,
			relationships: testRelationships,
			output:        mapRelationshipTuplesToCLIOutput(t, testRelationships),
		},
		{
			name:          "filters out",
			filter:        "test",
			schema:        testSchema,
			relationships: append([]string{"foo/user:0#reader@foo/resource:0"}, testRelationships...),
			output:        mapRelationshipTuplesToCLIOutput(t, testRelationships),
		},
		{
			name:          "allows empty backup",
			filter:        "test",
			schema:        testSchema,
			relationships: nil,
			output:        nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt := tt
			t.Parallel()

			cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t, zedtesting.StringFlag{FlagName: "prefix-filter", FlagValue: tt.filter})
			backupName := createTestBackup(t, tt.schema, tt.relationships)
			f, err := os.CreateTemp(t.TempDir(), "parse-output")
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = f.Close()
				_ = os.Remove(f.Name())
			})

			err = backupParseRelsCmdFunc(cmd, f, []string{backupName})
			require.NoError(t, err)

			lines := readLines(t, f.Name())
			require.Equal(t, tt.output, lines)
		})
	}
}

func TestBackupParseRevisionCmdFunc(t *testing.T) {
	t.Parallel()
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t, zedtesting.StringFlag{FlagName: "prefix-filter", FlagValue: "test"})
	backupName := createTestBackup(t, testSchema, testRelationships)
	f, err := os.CreateTemp(t.TempDir(), "parse-output")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	})

	err = backupParseRevisionCmdFunc(cmd, f, []string{backupName})
	require.NoError(t, err)

	lines := readLines(t, f.Name())
	require.Equal(t, []string{"test"}, lines)
}

func TestBackupParseSchemaCmdFunc(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		filter        string
		rewriteLegacy bool
		schema        string
		output        []string
		err           string
	}{
		{
			name:   "basic schema test",
			filter: "test",
			schema: testSchema,
			output: strings.Split(testSchema, "\n"),
		},
		{
			name:   "filters schema definitions",
			filter: "test",
			schema: "definition test/user {}\n\ndefinition foo/user {}",
			output: []string{"definition test/user {}"},
		},
		{
			name:          "rewrites short relations",
			filter:        "",
			rewriteLegacy: true,
			schema:        "definition user {relation aa: user}",
			output:        []string{"definition user {", "/* deleted short relation name */"},
		},
		{
			name:          "rewrites legacy missing allowed types",
			filter:        "",
			rewriteLegacy: true,
			schema:        "definition user { relation foo /* missing allowed types */}",
			output:        []string{"definition user {", "/* deleted missing allowed type error */"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt := tt
			t.Parallel()

			cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
				zedtesting.StringFlag{FlagName: "prefix-filter", FlagValue: tt.filter},
				zedtesting.BoolFlag{FlagName: "rewrite-legacy", FlagValue: tt.rewriteLegacy})
			backupName := createTestBackup(t, tt.schema, nil)
			f, err := os.CreateTemp(t.TempDir(), "parse-output")
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = f.Close()
				_ = os.Remove(f.Name())
			})

			err = backupParseSchemaCmdFunc(cmd, f, []string{backupName})
			require.NoError(t, err)

			lines := readLines(t, f.Name())
			require.Equal(t, tt.output, lines)
		})
	}
}

func TestBackupCreateCmdFunc(t *testing.T) {
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "prefix-filter"},
		zedtesting.BoolFlag{FlagName: "rewrite-legacy"},
		zedtesting.UintFlag32{FlagName: "page-limit"},
		zedtesting.StringFlag{FlagName: "token"},
		zedtesting.StringFlag{FlagName: "certificate-path"},
		zedtesting.StringFlag{FlagName: "endpoint"},
		zedtesting.BoolFlag{FlagName: "insecure"},
		zedtesting.BoolFlag{FlagName: "no-verify-ca"})

	ctx := t.Context()
	srv := zedtesting.NewTestServer(ctx, t)
	go func() {
		// NOTE: we don't assert anything about the error
		// here because there isn't a good time or place to do so.
		_ = srv.Run(ctx)
	}()
	conn, err := srv.GRPCDialContext(ctx)
	require.NoError(t, err)

	originalClient := client.NewClient
	t.Cleanup(func() {
		client.NewClient = originalClient
	})

	client.NewClient = zedtesting.ClientFromConn(conn)

	c, err := zedtesting.ClientFromConn(conn)(cmd)
	require.NoError(t, err)

	_, err = c.WriteSchema(ctx, &v1.WriteSchemaRequest{Schema: testSchema})
	require.NoError(t, err)

	update := &v1.WriteRelationshipsRequest{}
	testRel := "test/resource:1#reader@test/user:%d"
	expectedRels := make([]string, 0, 100)
	for i := range 100 {
		relString := fmt.Sprintf(testRel, i)
		update.Updates = append(update.Updates, &v1.RelationshipUpdate{
			Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: tuple.MustParseV1Rel(relString),
		})
		expectedRels = append(expectedRels, relString)
	}
	resp, err := c.WriteRelationships(ctx, update)
	require.NoError(t, err)

	t.Run("successful backup", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), uuid.NewString())
		err = backupCreateCmdFunc(cmd, []string{f})
		require.NoError(t, err)

		validateBackup(t, f, testSchema, resp.WrittenAt, expectedRels)
		// validate progress file is deleted after successful backup
		require.NoFileExists(t, f+".bak")
	})

	t.Run("fails if backup without progress file exists", func(t *testing.T) {
		tempFile := filepath.Join(t.TempDir(), uuid.NewString())
		f, err := os.Create(tempFile)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = f.Close()
			_ = os.Remove(f.Name())
		})

		err = backupCreateCmdFunc(cmd, []string{tempFile})
		require.ErrorContains(t, err, "already exists")
	})

	t.Run("derives backup file name from context if not provided", func(t *testing.T) {
		ds := client.DefaultStorage
		t.Cleanup(func() {
			client.DefaultStorage = ds
		})

		cfg := storage.Config{CurrentToken: "my-test"}
		cfgBytes, err := json.Marshal(cfg)
		require.NoError(t, err)

		testContextPath := filepath.Join(t.TempDir(), "config.json")
		err = os.WriteFile(testContextPath, cfgBytes, 0o600)
		require.NoError(t, err)

		name := uuid.NewString()
		client.DefaultStorage = func() (storage.ConfigStore, storage.SecretStore) {
			return &testConfigStore{currentToken: name},
				&testSecretStore{token: storage.Token{Name: name}}
		}
		err = backupCreateCmdFunc(cmd, nil)
		require.NoError(t, err)

		currentPath, err := os.Executable()
		require.NoError(t, err)
		exPath := filepath.Dir(currentPath)
		expectedBackupFile := filepath.Join(exPath, name+".zedbackup")
		require.FileExists(t, expectedBackupFile)
		validateBackup(t, expectedBackupFile, testSchema, resp.WrittenAt, expectedRels)
	})

	t.Run("truncates progress marker if it existed but backup did not", func(t *testing.T) {
		streamClient, err := c.ExportBulkRelationships(ctx, &v1.ExportBulkRelationshipsRequest{
			Consistency: &v1.Consistency{
				Requirement: &v1.Consistency_AtExactSnapshot{
					AtExactSnapshot: resp.WrittenAt,
				},
			},
			OptionalLimit: 1,
		})
		require.NoError(t, err)

		streamResp, err := streamClient.Recv()
		require.NoError(t, err)
		_ = streamClient.CloseSend()

		f := filepath.Join(t.TempDir(), uuid.NewString())
		lockFileName := f + ".lock"
		err = os.WriteFile(lockFileName, []byte(streamResp.AfterResultCursor.Token), 0o600)
		require.NoError(t, err)

		err = backupCreateCmdFunc(cmd, []string{f})
		require.NoError(t, err)

		// we know it did its work because it imported the 100 relationships regardless of the progress file
		validateBackup(t, f, testSchema, resp.WrittenAt, expectedRels)
		require.NoFileExists(t, lockFileName)
	})

	t.Run("resumes backup if marker file exists", func(t *testing.T) {
		streamClient, err := c.ExportBulkRelationships(ctx, &v1.ExportBulkRelationshipsRequest{
			Consistency: &v1.Consistency{
				Requirement: &v1.Consistency_AtExactSnapshot{
					AtExactSnapshot: resp.WrittenAt,
				},
			},
			OptionalLimit: 90,
		})
		require.NoError(t, err)

		streamResp, err := streamClient.Recv()
		require.NoError(t, err)
		_ = streamClient.CloseSend()

		f := filepath.Join(t.TempDir(), uuid.NewString())

		// do an initial backup to have the OCF metadata in place, it will also import the 100 rels
		err = backupCreateCmdFunc(cmd, []string{f})
		require.NoError(t, err)
		require.FileExists(t, f)

		lockFileName := f + ".lock"
		err = os.WriteFile(lockFileName, []byte(streamResp.AfterResultCursor.Token), 0o600)
		require.NoError(t, err)

		// run backup again, this time with an existing backup file and progress file
		err = backupCreateCmdFunc(cmd, []string{f})
		require.NoError(t, err)
		require.NoFileExists(t, lockFileName)

		// we know it did its work because we created a progress file at relationship 90, so we will get
		// a backup with 100 rels from the original import + the last 10 rels repeated again (110 in total)
		validateBackupWithFunc(t, f, testSchema, resp.WrittenAt, expectedRels, func(t testing.TB, expected, received []string) {
			require.Len(t, received, 110)
			receivedSet := mapz.NewSet(received...)
			expectedSet := mapz.NewSet(expected...)

			require.Equal(t, 100, receivedSet.Len())
			require.True(t, receivedSet.Equal(expectedSet))

			for i, s := range received[100:] {
				require.Equal(t, fmt.Sprintf(testRel, i+90), s)
			}
		})
	})
}

type testConfigStore struct {
	storage.ConfigStore
	currentToken string
}

func (tcs testConfigStore) Get() (storage.Config, error) {
	return storage.Config{CurrentToken: tcs.currentToken}, nil
}

func (tcs testConfigStore) Exists() (bool, error) {
	return true, nil
}

type testSecretStore struct {
	storage.SecretStore
	token storage.Token
}

func (tss testSecretStore) Get() (storage.Secrets, error) {
	return storage.Secrets{Tokens: []storage.Token{tss.token}}, nil
}

func validateBackup(t testing.TB, backupFileName string, schema string, token *v1.ZedToken, expected []string) {
	t.Helper()

	validateBackupWithFunc(t, backupFileName, schema, token, expected, func(t testing.TB, expected, received []string) {
		require.ElementsMatch(t, expected, received)
	})
}

func validateBackupWithFunc(t testing.TB, backupFileName, schema string, token *v1.ZedToken, expected []string,
	validateRels func(t testing.TB, expected, received []string),
) {
	t.Helper()

	d, closer, err := decoderFromArgs(backupFileName)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = d.Close()
		_ = closer.Close()
	})

	require.Equal(t, schema, d.Schema())
	require.Equal(t, token.Token, d.ZedToken().Token)
	var received []string
	for {
		rel, err := d.Next()
		if rel == nil {
			break
		}

		require.NoError(t, err)
		received = append(received, tuple.MustV1StringRelationship(rel))
	}

	validateRels(t, expected, received)
}

func TestBackupRestoreCmdFunc(t *testing.T) {
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "prefix-filter", FlagValue: "test"},
		zedtesting.BoolFlag{FlagName: "rewrite-legacy"},
		zedtesting.StringFlag{FlagName: "conflict-strategy", FlagValue: "fail"},
		zedtesting.BoolFlag{FlagName: "disable-retries"},
		zedtesting.UintFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.UintFlag{FlagName: "batches-per-transaction", FlagValue: 10},
		zedtesting.DurationFlag{FlagName: "request-timeout"},
	)
	backupName := createTestBackup(t, testSchema, testRelationships)

	ctx := t.Context()
	srv := zedtesting.NewTestServer(ctx, t)
	go func() {
		// NOTE: we don't assert anything about the error
		// here because there isn't a good time or place to do so.
		_ = srv.Run(ctx)
	}()
	conn, err := srv.GRPCDialContext(ctx)
	require.NoError(t, err)

	originalClient := client.NewClient
	defer func() {
		client.NewClient = originalClient
	}()

	client.NewClient = zedtesting.ClientFromConn(conn)

	c, err := zedtesting.ClientFromConn(conn)(cmd)
	require.NoError(t, err)
	err = backupRestoreCmdFunc(cmd, []string{backupName})
	require.NoError(t, err)

	resp, err := c.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	require.NoError(t, err)
	require.Equal(t, testSchema, resp.SchemaText)

	rrCli, err := c.ReadRelationships(ctx, &v1.ReadRelationshipsRequest{
		Consistency: &v1.Consistency{
			Requirement: &v1.Consistency_FullyConsistent{
				FullyConsistent: true,
			},
		},
		RelationshipFilter: &v1.RelationshipFilter{
			ResourceType: "test/resource",
		},
	})
	require.NoError(t, err)

	rrResp, err := rrCli.Recv()
	require.NoError(t, err)

	require.NoError(t, rrCli.CloseSend())
	require.Equal(t, "test/resource:1#reader@test/user:1", tuple.MustV1StringRelationship(rrResp.Relationship))
}

func TestAddSizeErrInfo(t *testing.T) {
	tcs := []struct {
		name          string
		err           error
		expectedError string
	}{
		{
			name:          "error is nil",
			err:           nil,
			expectedError: "",
		},
		{
			name:          "error is not a size error",
			err:           errors.New("some error"),
			expectedError: "some error",
		},
		{
			name:          "error has correct code, wrong message",
			err:           status.New(codes.ResourceExhausted, "foobar").Err(),
			expectedError: "foobar",
		},
		{
			name:          "error has correct message, wrong code",
			err:           status.New(codes.Unauthenticated, "received message larger than max").Err(),
			expectedError: "received message larger than max",
		},
		{
			name:          "error has correct code and message",
			err:           status.New(codes.ResourceExhausted, "received message larger than max").Err(),
			expectedError: "set flag --max-message-size=bytecounthere",
		},
		{
			name:          "error has correct code and message with additional info",
			err:           status.New(codes.ResourceExhausted, "received message larger than max (1234 vs. 45)").Err(),
			expectedError: "set flag --max-message-size=2468",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			err := addSizeErrInfo(tc.err)
			if tc.expectedError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.expectedError)
			}
		})
	}
}

func TestTakeBackupRecoversFromRetryableErrors(t *testing.T) {
	firstRels := []*v1.Relationship{{
		Resource: &v1.ObjectReference{ObjectType: "resource", ObjectId: "foo"},
		Relation: "view",
		Subject: &v1.SubjectReference{
			Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "jim"},
		},
	}}
	cursor := &v1.Cursor{Token: "a token"}
	secondRels := []*v1.Relationship{{
		Resource: &v1.ObjectReference{ObjectType: "resource", ObjectId: "bar"},
		Relation: "view",
		Subject: &v1.SubjectReference{
			Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "jim"},
		},
	}}
	client := &mockClientForBackup{
		t: t,
		schemaCalls: []func() (*v1.ReadSchemaResponse, error){
			func() (*v1.ReadSchemaResponse, error) {
				return &v1.ReadSchemaResponse{
					ReadAt: &v1.ZedToken{Token: "init"},
				}, nil
			},
		},
		recvCalls: []func() (*v1.ExportBulkRelationshipsResponse, error){
			func() (*v1.ExportBulkRelationshipsResponse, error) {
				return &v1.ExportBulkRelationshipsResponse{
					Relationships: firstRels,
					// Need to test that this cursor is supplied
					AfterResultCursor: cursor,
				}, nil
			},
			func() (*v1.ExportBulkRelationshipsResponse, error) {
				// Return a retryable error
				return nil, status.Error(codes.Unavailable, "i fell over")
			},
			func() (*v1.ExportBulkRelationshipsResponse, error) {
				return &v1.ExportBulkRelationshipsResponse{
					Relationships: secondRels,
					AfterResultCursor: &v1.Cursor{
						Token: "some other token",
					},
				}, nil
			},
		},
		exportCalls: []func(t *testing.T, req *v1.ExportBulkRelationshipsRequest){
			// Initial request
			func(_ *testing.T, _ *v1.ExportBulkRelationshipsRequest) {},
			// The retried request - asserting that it's called with the cursor
			func(t *testing.T, req *v1.ExportBulkRelationshipsRequest) {
				require.NotNil(t, req)
				require.NotNil(t, req.OptionalCursor, "cursor should be set on retry")
				require.Equal(t, req.OptionalCursor.Token, cursor.Token, "cursor token does not match expected, got %s", req.OptionalCursor.Token)
			},
		},
	}

	encoder := &backupformat.MockEncoder{}

	err := takeBackup(t.Context(), client, encoder, "ignored", "", 0)
	require.NoError(t, err)

	require.True(t, encoder.Complete, "expecting encoder to be marked complete")
	require.Len(t, encoder.Relationships, 2, "expecting two rels in the realized list")
	require.Equal(t, encoder.Relationships[0].Resource.ObjectId, "foo")
	require.Equal(t, encoder.Relationships[1].Resource.ObjectId, "bar")

	client.assertAllRecvCalls()
}

type mockClientForBackup struct {
	client.Client
	grpc.ServerStreamingClient[v1.ExportBulkRelationshipsResponse]
	t *testing.T
	backupformat.OcfEncoder
	schemaCalls      []func() (*v1.ReadSchemaResponse, error)
	schemaCallsIndex int
	recvCalls        []func() (*v1.ExportBulkRelationshipsResponse, error)
	recvCallIndex    int
	// exportCalls provides a handle on the calls made to ExportBulkRelationships,
	// allowing for assertions to be made against those calls.
	exportCalls      []func(t *testing.T, req *v1.ExportBulkRelationshipsRequest)
	exportCallsIndex int
}

func (m *mockClientForBackup) Recv() (*v1.ExportBulkRelationshipsResponse, error) {
	// If we've run through all our calls, return an EOF
	if m.recvCallIndex == len(m.recvCalls) {
		return nil, io.EOF
	}
	recvCall := m.recvCalls[m.recvCallIndex]
	m.recvCallIndex++
	return recvCall()
}

func (m *mockClientForBackup) ReadSchema(ctx context.Context, req *v1.ReadSchemaRequest, opts ...grpc.CallOption) (*v1.ReadSchemaResponse, error) {
	if m.schemaCalls == nil {
		// If the caller doesn't supply any calls, pass through
		return m.ReadSchema(ctx, req, opts...)
	} else if m.schemaCallsIndex == len(m.schemaCalls) {
		// If invoked too many times, fail the test
		m.t.FailNow()
		return nil, nil
	}

	schemaCall := m.schemaCalls[m.schemaCallsIndex]
	m.schemaCallsIndex++
	return schemaCall()
}

func (m *mockClientForBackup) ExportBulkRelationships(_ context.Context, req *v1.ExportBulkRelationshipsRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[v1.ExportBulkRelationshipsResponse], error) {
	if m.exportCalls == nil {
		// If the caller doesn't supply exportCalls, pass through
		return m, nil
	} else if m.exportCallsIndex == len(m.exportCalls) {
		// If invoked too many times, fail the test
		m.t.FailNow()
		return m, nil
	}
	exportCall := m.exportCalls[m.exportCallsIndex]
	m.exportCallsIndex++
	exportCall(m.t, req)
	return m, nil
}

// assertAllRecvCalls asserts that the number of invocations is as expected
func (m *mockClientForBackup) assertAllRecvCalls() {
	require.Equal(m.t, len(m.recvCalls), m.recvCallIndex, "the number of provided recvCalls should match the number of invocations")
}
